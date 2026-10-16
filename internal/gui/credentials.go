package gui

import (
	"encoding/json"
	"os"

	"yati/internal/config"
	"yati/internal/localfile"
)

func saveToken(token string) error {
	path := os.Getenv("YATI_CONFIG")
	if path == "" {
		path = config.DefaultConfigPath()
	}
	values := make(map[string]json.RawMessage)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if values == nil {
		values = make(map[string]json.RawMessage)
	}
	encoded, err := json.Marshal(token)
	if err != nil {
		return err
	}
	values["token"] = encoded
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	return localfile.WritePrivate(path, append(data, '\n'))
}
