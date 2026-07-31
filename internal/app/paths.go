package app

import (
	"os"
	"path/filepath"
)

type Paths struct {
	DataRoot     string `json:"dataRoot"`
	RouterConfig string `json:"routerConfig"`
	BackupRoot   string `json:"backupRoot"`
	AgyConfig    string `json:"agyConfig"`
	Executable   string `json:"executable"`
}

func DefaultPaths() (Paths, error) {
	var root string
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		root = filepath.Join(local, "AgentStack")
	} else {
		config, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, err
		}
		root = filepath.Join(config, "agentstack")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	agyCurrent := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	agyAlternate := filepath.Join(home, ".gemini", "antigravity-cli", "mcp_config.json")
	agy := agyCurrent
	if _, err := os.Stat(agyCurrent); os.IsNotExist(err) {
		if _, altErr := os.Stat(agyAlternate); altErr == nil {
			agy = agyAlternate
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return Paths{}, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		DataRoot:     root,
		RouterConfig: filepath.Join(root, "router", "router.json"),
		BackupRoot:   filepath.Join(root, "backups"),
		AgyConfig:    agy,
		Executable:   executable,
	}, nil
}
