package cmp_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pluginclient "github.com/argoproj/argo-cd/v3/cmpserver/apiclient"
	"github.com/argoproj/argo-cd/v3/test"
	"github.com/argoproj/argo-cd/v3/util/cmp"
	"github.com/argoproj/argo-cd/v3/util/io/files"
)

type streamMock struct {
	messages chan *pluginclient.AppStreamRequest
	done     chan bool
}

func (m *streamMock) Recv() (*pluginclient.AppStreamRequest, error) {
	select {
	case message := <-m.messages:
		return message, nil
	case <-m.done:
		return nil, io.EOF
	case <-time.After(500 * time.Millisecond):
		return nil, errors.New("timeout receiving message mock")
	}
}

func (m *streamMock) Send(message *pluginclient.AppStreamRequest) error {
	m.messages <- message
	return nil
}

func newStreamMock() *streamMock {
	messagesCh := make(chan *pluginclient.AppStreamRequest)
	doneCh := make(chan bool)
	return &streamMock{
		messages: messagesCh,
		done:     doneCh,
	}
}

func TestReceiveApplicationStream(t *testing.T) {
	t.Run("will receive the application stream successfully", func(t *testing.T) {
		// given
		streamMock := newStreamMock()
		appDir := filepath.Join(getTestDataDir(t), "app")
		workdir, err := files.CreateTempDir("")
		require.NoError(t, err)
		defer func() {
			close(streamMock.messages)
			os.RemoveAll(workdir)
		}()
		go streamMock.sendFile(t.Context(), t, appDir, streamMock, []string{"env1", "env2"}, []string{"DUMMY.md", "dum*"})

		// when
		env, err := cmp.ReceiveRepoStream(t.Context(), streamMock, workdir, false)

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, workdir)
		files, err := os.ReadDir(workdir)
		require.NoError(t, err)
		require.Len(t, files, 2)
		names := []string{}
		for _, f := range files {
			names = append(names, f.Name())
		}
		assert.Contains(t, names, "README.md")
		assert.Contains(t, names, "applicationset")
		assert.NotContains(t, names, "DUMMY.md")
		assert.NotContains(t, names, "dummy")
		assert.NotNil(t, env)
	})
}

// capturingSender records every AppStreamRequest sent through it.
type capturingSender struct {
	sent []*pluginclient.AppStreamRequest
}

func (c *capturingSender) Send(req *pluginclient.AppStreamRequest) error {
	c.sent = append(c.sent, req)
	return nil
}

func TestSendMetadataOnlyStream(t *testing.T) {
	t.Parallel()

	t.Run("sends exactly one metadata message with no checksum or size", func(t *testing.T) {
		t.Parallel()
		rootDir := t.TempDir()
		appDir := filepath.Join(rootDir, "myapp")
		require.NoError(t, os.Mkdir(appDir, 0o755))

		sender := &capturingSender{}
		err := cmp.SendMetadataOnlyStream(t.Context(), appDir, rootDir, sender, []string{"FOO=bar"})
		require.NoError(t, err)

		require.Len(t, sender.sent, 1, "should send exactly one message")
		meta := sender.sent[0].GetMetadata()
		require.NotNil(t, meta, "message should carry metadata")
		assert.Equal(t, "myapp", meta.AppName)
		assert.Equal(t, "", meta.Checksum, "no checksum for metadata-only stream")
		assert.EqualValues(t, 0, meta.Size_, "no size for metadata-only stream")
		require.Len(t, meta.Env, 1)
		assert.Equal(t, "FOO", meta.Env[0].Name)
		assert.Equal(t, "bar", meta.Env[0].Value)
	})
}

func TestReceiveMetadataOnlyStream(t *testing.T) {
	t.Parallel()

	t.Run("returns metadata from the first stream message", func(t *testing.T) {
		t.Parallel()
		m := newStreamMock()
		go func() {
			m.messages <- &pluginclient.AppStreamRequest{
				Request: &pluginclient.AppStreamRequest_Metadata{
					Metadata: &pluginclient.ManifestRequestMetadata{
						AppName:    "myapp",
						AppRelPath: ".",
						Env:        []*pluginclient.EnvEntry{{Name: "FOO", Value: "bar"}},
					},
				},
			}
		}()

		meta, err := cmp.ReceiveMetadataOnlyStream(t.Context(), m)
		require.NoError(t, err)
		require.NotNil(t, meta)
		assert.Equal(t, "myapp", meta.AppName)
		require.Len(t, meta.Env, 1)
		assert.Equal(t, "bar", meta.Env[0].Value)
	})

	t.Run("returns error when metadata is nil", func(t *testing.T) {
		t.Parallel()
		m := newStreamMock()
		go func() {
			// Send a file chunk instead of metadata — metadata will be nil
			m.messages <- &pluginclient.AppStreamRequest{
				Request: &pluginclient.AppStreamRequest_File{
					File: &pluginclient.File{Chunk: []byte("data")},
				},
			}
		}()

		_, err := cmp.ReceiveMetadataOnlyStream(t.Context(), m)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "metadata is nil")
	})

	t.Run("returns error when stream ends without sending a message", func(t *testing.T) {
		t.Parallel()
		m := newStreamMock()
		go func() {
			m.done <- true
		}()

		_, err := cmp.ReceiveMetadataOnlyStream(t.Context(), m)
		require.Error(t, err)
	})
}

func (m *streamMock) sendFile(ctx context.Context, t *testing.T, basedir string, sender cmp.StreamSender, env []string, excludedGlobs []string) {
	t.Helper()
	defer func() {
		m.done <- true
	}()
	err := cmp.SendRepoStream(ctx, basedir, basedir, sender, env, excludedGlobs)
	require.NoError(t, err)
}

// getTestDataDir will return the full path of the testdata dir
// under the running test folder.
func getTestDataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(test.GetTestDir(t), "testdata")
}
