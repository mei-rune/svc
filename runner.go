package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
)

var configPath string

func init() {
	flag.StringVar(&configPath, "config", "", "the config file path")
}

func getDefaultConfigPath() (string, error) {
	fullexecpath, err := os.Executable()
	if err != nil {
		return "", err
	}

	dir, execname := filepath.Split(fullexecpath)
	ext := filepath.Ext(execname)
	name := execname[:len(execname)-len(ext)]

	return filepath.Join(dir, name+".json"), nil
}

func readConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conf := &Config{}

	r := json.NewDecoder(f)
	err = r.Decode(&conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

func createProgram(configPath string) (*Program, error) {
	if configPath == "" {
		pa, err := getDefaultConfigPath()
		if err != nil {
			return nil, err
		}
		configPath = pa
	}

	config, err := readConfig(configPath)
	if err != nil {
		return nil, err
	}

	svcConfig := &service.Config{
		Name:        config.Name,
		DisplayName: config.DisplayName,
		Description: config.Description,
	}

	prg := &Program{
		Config: config,
	}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		return nil, err
	}
	prg.service = s
	return prg, nil
}

type ControlAction struct {
	method string
}

func (ca *ControlAction) Flags(fs *flag.FlagSet) *flag.FlagSet {
	return fs
}

func (ca *ControlAction) Run(args []string) error {
	prg, err := createProgram(configPath)
	if err != nil {
		return err
	}
	err = service.Control(prg.service, ca.method)
	if err != nil {
		log.Printf("Valid actions: %q\n", service.ControlAction)
		return err
	}
	return nil
}

type RunAction struct{}

func (ca *RunAction) Flags(fs *flag.FlagSet) *flag.FlagSet {
	return fs
}

func (ca *RunAction) Run(args []string) error {
	prg, err := createProgram(configPath)
	if err != nil {
		return err
	}
	return prg.service.Run()
}

func init() {
	for _, method := range service.ControlAction {
		On(method, "", &ControlAction{
			method: method,
		})
	}
	On("service", "", &RunAction{})
}

func main() {
	Parse()

	Run()
}
