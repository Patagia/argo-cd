package plugin

import (
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/argoproj/argo-cd/v3/common"
	"github.com/argoproj/argo-cd/v3/reposerver/apiclient"
	configUtil "github.com/argoproj/argo-cd/v3/util/config"
)

const (
	ConfigManagementPluginKind string = "ConfigManagementPlugin"
)

type PluginConfig struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            PluginConfigSpec  `json:"spec"`
}

type PluginConfigSpec struct {
	Version string `json:"version"`
	// Fetch is an optional command that the plugin runs to retrieve the source before generate.
	// When set, ArgoCD skips its own git/OCI fetch and sends only metadata to the plugin sidecar;
	// the fetch command is responsible for downloading the source into the working directory.
	//
	// Constraint: a plugin with spec.fetch configured must be explicitly referenced by name in
	// the Application resource (spec.source.plugin.name). Auto-discovery is not supported because
	// ArgoCD cannot stream repository files to discover the plugin when no fetch has occurred yet.
	//
	// When spec.fetch is set, spec.init is skipped (fetch subsumes the setup role).
	Fetch            Command    `json:"fetch,omitempty"`
	Init             Command    `json:"init,omitempty"`
	Generate         Command    `json:"generate"`
	Discover         Discover   `json:"discover"`
	Parameters       Parameters `yaml:"parameters"`
	PreserveFileMode bool       `json:"preserveFileMode,omitempty"`
	ProvideGitCreds  bool       `json:"provideGitCreds,omitempty"`
}

// Discover holds find and fileName
type Discover struct {
	Find     Find   `json:"find"`
	FileName string `json:"fileName"`
}

func (d Discover) IsDefined() bool {
	return d.FileName != "" || d.Find.Glob != "" || len(d.Find.Command.Command) > 0
}

// Command holds binary path and arguments list
type Command struct {
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// Find holds find command or glob pattern
type Find struct {
	Command
	Glob string `json:"glob"`
}

// Parameters holds static and dynamic configurations
type Parameters struct {
	Static  []*apiclient.ParameterAnnouncement `yaml:"static"`
	Dynamic Command                            `yaml:"dynamic"`
}

// Dynamic hold the dynamic announcements for CMP's
type Dynamic struct {
	Command
}

func ReadPluginConfig(filePath string) (*PluginConfig, error) {
	path := fmt.Sprintf("%s/%s", strings.TrimRight(filePath, "/"), common.PluginConfigFileName)

	var config PluginConfig
	err := configUtil.UnmarshalLocalFile(path, &config)
	if err != nil {
		return nil, err
	}

	err = ValidatePluginConfig(config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func ValidatePluginConfig(config PluginConfig) error {
	if config.Metadata.Name == "" {
		return errors.New("invalid plugin configuration file. metadata.name should be non-empty")
	}
	if config.Kind != ConfigManagementPluginKind {
		return fmt.Errorf("invalid plugin configuration file. kind should be %s, found %s", ConfigManagementPluginKind, config.Kind)
	}
	if len(config.Spec.Generate.Command) == 0 {
		return errors.New("invalid plugin configuration file. spec.generate command should be non-empty")
	}
	// A plugin with spec.fetch cannot use auto-discovery: ArgoCD has no files to stream to the
	// plugin for matching before the fetch has occurred. The Application must reference the plugin
	// by name (spec.source.plugin.name). Configuring spec.discover alongside spec.fetch is
	// therefore invalid and would silently never match any application.
	if len(config.Spec.Fetch.Command) > 0 && config.Spec.Discover.IsDefined() {
		return errors.New("invalid plugin configuration file. spec.fetch and spec.discover are mutually exclusive: a fetch-capable plugin must be referenced by name (spec.source.plugin.name) in the Application resource")
	}
	// discovery field is optional as apps can now specify plugin names directly
	return nil
}

func (cfg *PluginConfig) Address() string {
	var address string
	pluginSockFilePath := common.GetPluginSockFilePath()
	if cfg.Spec.Version != "" {
		address = fmt.Sprintf("%s/%s-%s.sock", pluginSockFilePath, cfg.Metadata.Name, cfg.Spec.Version)
	} else {
		address = fmt.Sprintf("%s/%s.sock", pluginSockFilePath, cfg.Metadata.Name)
	}
	return address
}
