package plugin

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/hashicorp/go-plugin"

	"github.com/ethereum/go-ethereum/plugin/helloWorld"
	"github.com/naoina/toml"
)

type Version string
type RawConfiguration []byte

// This is to describe a plugin
//
// Information is used to discover the plugin binary and verify its integrity
// before forking a process running the plugin
type PluginDefinition struct {
	Name string `json:"name" toml:""`
	// the semver version of the plugin
	Version Version `json:"version" toml:""`
	// plugin configuration in a form of map/slice/string
	Config interface{} `json:"config,omitempty" toml:",omitempty"`
}

func (m *PluginDefinition) ReadConfig() ([]byte, error) {
	if m.Config == nil {
		return []byte{}, nil
	}
	switch k := reflect.TypeOf(m.Config).Kind(); k {
	case reflect.Map, reflect.Slice:
		return json.Marshal(m.Config)
	case reflect.String:
		configStr := m.Config.(string)
		u, err := url.Parse(configStr)
		if err != nil { // just return as is
			return []byte(configStr), nil
		}
		switch s := u.Scheme; s {
		case "file":
			return ioutil.ReadFile(filepath.Join(u.Host, u.Path))
		case "env": // config string in an env variable
			varName := u.Host
			isFile := u.Query().Get("type") == "file"
			if v, ok := os.LookupEnv(varName); ok {
				if isFile {
					m.Config = v
					return ioutil.ReadFile(v)
				} else {
					return []byte(v), nil
				}
			} else {
				return nil, fmt.Errorf("env variable %s not found", varName)
			}
		default:
			return []byte(configStr), nil
		}
	default:
		return nil, fmt.Errorf("unsupported type of config [%s]", k)
	}
}

// return remote folder storing the plugin distribution file and signature file
//
// e.g.: my-plugin/v1.0.0/darwin-amd64
func (m *PluginDefinition) RemotePath() string {
	return fmt.Sprintf("%s/v%s/%s-%s", m.Name, m.Version, runtime.GOOS, runtime.GOARCH)
}

// return plugin name and version
func (m *PluginDefinition) FullName() string {
	return fmt.Sprintf("%s-%s", m.Name, m.Version)
}

// return plugin distribution file name
func (m *PluginDefinition) DistFileName() string {
	return fmt.Sprintf("%s.zip", m.FullName())
}

// return plugin distribution signature file name
func (m *PluginDefinition) SignatureFileName() string {
	return fmt.Sprintf("%s.sig", m.FullName())
}

const (
	HelloWorldPluginInterfaceName = PluginInterfaceName("helloworld") // lower-case always
)

// define additional plugins here
var pluginProviders = map[PluginInterfaceName]plugin.PluginSet{
	HelloWorldPluginInterfaceName: {
		helloWorld.ConnectorName: &helloWorld.PluginConnector{},
	},
}

// must be always be lowercase when define constants
// as when unmarshaling from config, value will be case-lowered
type PluginInterfaceName string

// When this is used as a key in map. This function is not invoked.
func (p *PluginInterfaceName) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = PluginInterfaceName(strings.ToLower(v))
	return nil
}

func (p *PluginInterfaceName) UnmarshalTOML(data []byte) error {
	var v string
	if err := toml.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = PluginInterfaceName(strings.ToLower(v))
	return nil
}

func (p *PluginInterfaceName) UnmarshalText(data []byte) error {
	*p = PluginInterfaceName(strings.ToLower(string(data)))
	return nil
}

func (p PluginInterfaceName) String() string {
	return string(p)
}

// this defines plugins used in the geth node
type Settings struct {
	BaseDir       EnvironmentAwaredValue                   `json:"baseDir" toml:""`
	CentralConfig *PluginCentralConfiguration              `json:"central" toml:"Central"`
	Providers     map[PluginInterfaceName]PluginDefinition `json:"providers" toml:""`
}

func (s *Settings) GetPluginDefinition(name PluginInterfaceName) (*PluginDefinition, bool) {
	m, ok := s.Providers[name]
	return &m, ok
}

func (s *Settings) SetDefaults() {
	if s.CentralConfig == nil {
		s.CentralConfig = quorumPluginCentralConfiguration
	}
}

type PluginCentralConfiguration struct {
	// To implement certificate pinning while communicating with PluginCentral
	// if it's empty, we skip cert pinning logic
	CertFingerprint    string `json:"certFingerprint" toml:""`
	BaseURL            string `json:"baseURL" toml:""`
	PublicKeyURI       string `json:"publicKeyURI" toml:""`
	InsecureSkipVerify bool   `json:"insecureSkipVerify" toml:""`
}

var quorumPluginCentralConfiguration = &PluginCentralConfiguration{
	CertFingerprint:    "13ae1ae0e6d0e70b6b9e17fcd02d821b23f491f5867fe1472363a5d98d44884f",
	BaseURL:            "https://plugins.goquorum.com",
	PublicKeyURI:       "/" + DefaultPublicKeyFile,
	InsecureSkipVerify: false,
}

// support URI format with 'env' scheme during JSON/TOML/TEXT unmarshalling
// e.g.: env://FOO_VAR means read a string value from FOO_VAR environment variable
type EnvironmentAwaredValue string

func (d *EnvironmentAwaredValue) UnmarshalJSON(data []byte) error {
	return d.unmarshal(data)
}

func (d *EnvironmentAwaredValue) UnmarshalTOML(data []byte) error {
	return d.unmarshal(data)
}

func (d *EnvironmentAwaredValue) UnmarshalText(data []byte) error {
	return d.unmarshal(data)
}

func (d *EnvironmentAwaredValue) unmarshal(data []byte) error {
	v := string(data)
	isString := strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"")
	if !isString {
		return fmt.Errorf("not a string")
	}
	v = strings.TrimFunc(v, func(r rune) bool {
		return r == '"'
	})
	if u, err := url.Parse(v); err == nil {
		switch u.Scheme {
		case "env":
			v = os.Getenv(u.Host)
		}
	}
	*d = EnvironmentAwaredValue(v)
	return nil
}

func (d EnvironmentAwaredValue) String() string {
	return string(d)
}
