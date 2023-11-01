package svc

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kardianos/service"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logWriter io.Writer
var executablePath string
var executableName string
var executableDir string

func init() {
	var err error
	executablePath, err = os.Executable()
	if err != nil {
		panic(err)
	}
	executableDir = filepath.Dir(executablePath)
	executableName = filepath.Base(executablePath)
}

func GetExecutableDir() string {
	return executableDir
}

func GetExecutableName() string {
	return executableName
}

func InitLogger(logFilePath string) error {
	if logWriter != nil {
		return errors.New("logWriter already initialized")
	}

	if logFilePath == "" {
		name := executableName
		if runtime.GOOS == "windows" {
			ext := filepath.Ext(name)
			name = strings.TrimSuffix(name, ext)
			logFilePath = filepath.Join(executableDir, name+".log")
		} else {
			logFilePath = filepath.Join("/var/log", name+".log")
		}
	}
	log.Println("log write to", logFilePath)

	logWriter = &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    500, // megabytes
		MaxBackups: 8,
		MaxAge:     1,     //days
		Compress:   false, // disabled by default
	}
	log.SetOutput(logWriter)
	return nil
}

func getDefaultConfigPath() (string, error) {
	ext := filepath.Ext(executableName)
	name := strings.TrimSuffix(executableName, ext)
	return filepath.Join(executableDir, name+".json"), nil
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

var createProgram func(method string) (*Program, error)

type ControlAction struct {
	method string
}

func (ca *ControlAction) Flags(fs *flag.FlagSet) *flag.FlagSet {
	return fs
}

func (ca *ControlAction) Run(args []string) error {
	prg, err := createProgram(ca.method)
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
	prg, err := createProgram("service")
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

func RunService() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "the config file path")
	createProgram = func(method string) (*Program, error) {
		return createProgramFromFile(configPath, method)
	}
	Parse()
	Run()
}

func RunServiceWith(config Config) {
	createProgram = func(method string) (*Program, error) {
		return createProgramWithConfig(config, method)
	}

	if runtime.GOOS == "windows" {
		InitLogger(filepath.Join(executableDir, config.Name+".log"))
	} else {
		InitLogger(filepath.Join("/var/log/", config.Name+".log"))
	}
	Parse()
	Run()
}

func createProgramFromFile(configPath, method string) (*Program, error) {
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

	if runtime.GOOS == "windows" {
		InitLogger(filepath.Join(executableDir, config.Name+".log"))
	} else {
		InitLogger(filepath.Join("/var/log/", config.Name+".log"))
	}
	return createProgramWithConfig(*config, method)
}

func createProgramWithConfig(config Config, method string) (*Program, error) {
	svcConfig := &service.Config{
		Name:        config.Name,
		DisplayName: config.DisplayName,
		Description: config.Description,
	}

	fullExec := config.Exec
	// Look for exec.
	// Verify home directory.
	if !filepath.IsAbs(fullExec) {
		var err error

		var retried = false
	retry:
		fullExec, err = exec.LookPath(filepath.Join(executableDir, config.Exec))
		if err != nil {
			fullExec, err = exec.LookPath(filepath.Join(config.Dir, config.Exec))
			if err != nil {
				fullExec, err = exec.LookPath(config.Exec)
				if err != nil {
					found := false
					for _, m := range service.ControlAction {
						if m == method {
							found = true
							break
						}
					}
					if found {
						err = nil
					}
				}
			}
		}

		if err != nil {
			if retried {
				log.Printf("Failed to find executable %q: %v", config.Exec, err)
				return nil, fmt.Errorf("Failed to find executable %q: %v", config.Exec, err)
			}

			updateOk, e := runUpdateOnce(config.Update)
			if !updateOk || e != nil {
				if e == nil {
					log.Printf("尝试从升级仓库中获取失败: no versions?")
				} else {
					log.Printf("没有找到可执行文件，尝试从升级仓库中获取失败: %v", e)
				}
				log.Printf("Failed to find executable %q: %v", config.Exec, err)
				return nil, fmt.Errorf("Failed to find executable %q: %v", config.Exec, err)
			}
			retried = true
			goto retry
		}
	}
	config.Exec = fullExec

	if config.Stdout != "" {
		if !filepath.IsAbs(config.Stdout) {
			if runtime.GOOS == "windows" {
				config.Stdout = filepath.Join(executableDir, config.Stdout)
			} else {
				config.Stdout = filepath.Join("/var/log/", config.Stdout)
			}
		}
	}
	if config.Stderr != "" && strings.HasPrefix(config.Stderr, "&") {
		if !filepath.IsAbs(config.Stderr) {
			if runtime.GOOS == "windows" {
				config.Stderr = filepath.Join(executableDir, config.Stderr)
			} else {
				config.Stderr = filepath.Join("/var/log/", config.Stderr)
			}
		}
	}

	prg := &Program{
		Config: config,
	}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Printf("create svc instance: %v", err)
		return nil, err
	}
	prg.service = s
	return prg, nil
}
