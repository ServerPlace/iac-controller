package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	BackendURL   string `json:"backend_url"`
}

func GetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".iac-controller", "config.json")
}

func Load() (*Config, error) {
	path := GetConfigPath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("configuração não encontrada. Execute 'iac-admin init'")
		}
		return nil, err
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("erro ao ler configuração: %w", err)
	}

	return &cfg, nil
}

func Initialize() error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("🔧 Configuração do IaC Admin CLI")
	fmt.Println()

	fmt.Print("Client ID: ")
	scanner.Scan()
	clientID := strings.TrimSpace(scanner.Text())

	fmt.Print("Client Secret: ")
	scanner.Scan()
	clientSecret := strings.TrimSpace(scanner.Text())

	fmt.Print("Backend URL ex: [https://iac-controller-<xxxxxxxxxxx>.us-central1.run.app]: ")
	scanner.Scan()
	backendURL := strings.TrimSpace(scanner.Text())
	if backendURL == "" {
		fmt.Print("Backend url is required")
		os.Exit(1)
	}

	cfg := Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		BackendURL:   backendURL,
	}

	path := GetConfigPath()
	os.MkdirAll(filepath.Dir(path), 0700)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo de configuração: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("erro ao salvar configuração: %w", err)
	}

	return nil
}
