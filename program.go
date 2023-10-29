package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/kardianos/service"
)

// Config is the runner app config structure.
type Config struct {
	Name, DisplayName, Description string

	Dir  string
	Exec string
	Args []string
	Env  []string

	Stderr, Stdout string
}

type Program struct {
	service service.Service

	*Config

	cmd *exec.Cmd
}

func (p *Program) Start(s service.Service) error {
	// Look for exec.
	// Verify home directory.
	fullExec, err := exec.LookPath(p.Exec)
	if err != nil {
		return fmt.Errorf("Failed to find executable %q: %v", p.Exec, err)
	}

	p.cmd = exec.Command(fullExec, p.Args...)
	p.cmd.Dir = p.Dir
	p.cmd.Env = append(os.Environ(), p.Env...)

	go p.run()
	return nil
}

func (p *Program) run() {
	log.Println("Starting ", p.DisplayName)
	defer func() {
		if service.Interactive() {
			p.Stop(p.service)
		} else {
			p.service.Stop()
		}
	}()


	if p.Stdout != "" {
		f, err := os.OpenFile(p.Stdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			log.Println("Failed to open std out %q: %v", p.Stdout, err)
			return
		}
		defer f.Close()
		p.cmd.Stdout = f
	}

	if p.Stderr != "" {
		if p.Stderr == "&stdout" {
			p.cmd.Stderr = p.cmd.Stdout
		} else {
			f, err := os.OpenFile(p.Stderr, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
			if err != nil {
				log.Printf("Failed to open std err %q: %v", p.Stderr, err)
				return
			}
			defer f.Close()
			p.cmd.Stderr = f
		}
	}

	err := p.cmd.Run()
	if err != nil {
		log.Println("Error running: %v", err)
	}
}

func (p *Program) Stop(s service.Service) error {
	log.Println("Stopping ", p.DisplayName)
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	if service.Interactive() {
		os.Exit(0)
	}
	return nil
}
