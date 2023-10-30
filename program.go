package svc

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime/debug"
	"time"

	"github.com/kardianos/service"
	"github.com/mei-rune/autoupdate"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config is the runner app config structure.
type Config struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`

	Update autoupdate.Options `json:"update"`

	Dir  string   `json:"dir"`
	Exec string   `json:"exec"`
	Args []string `json:"args"`
	Env  []string `json:"env"`

	Stderr string `json:"stderr"`
	Stdout string `json:"stdout"`
}

type Program struct {
	exit chan struct{}

	service service.Service

	Config Config

	cmd *exec.Cmd
}

func (p *Program) Start(s service.Service) error {
	p.exit = make(chan struct{})

	if p.Config.Update.BaseURL != "" {
		updater, err := autoupdate.NewUpdater(p.Config.Update)
		if err != nil {
			log.Println("启用自动升级功能失败,", err)
			return errors.New("启用自动升级功能失败," + err.Error())
		}
		go RunUpdate(updater, p.exit)

		log.Println("启用自动升级功能！")
	}

	go p.run()
	return nil
}

func (p *Program) run() {
	log.Println("Starting ", p.Config.DisplayName, ", isService =", !service.Interactive())
	defer func() {
		if o := recover(); o != nil {

			if logWriter != nil {
				fmt.Fprintln(logWriter, o)
				logWriter.Write(debug.Stack())
			} else {
				log.Println(o)
				log.Println(string(debug.Stack()))
			}
			if service.Interactive() {
				p.Stop(p.service)
			} else {
				p.service.Stop()
			}
		}
	}()

	for {
		isRunning := true
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-p.exit:
			timer.Stop()
			isRunning = false
		case <-timer.C:
		}

		if !isRunning {
			break
		}

		p.runOnce()
	}
}

func (p *Program) runOnce() {
	cmd := exec.Command(p.Config.Exec, p.Config.Args...)
	cmd.Dir = p.Config.Dir
	cmd.Env = append(os.Environ(), p.Config.Env...)

	if p.Config.Stdout != "" {
		w := &lumberjack.Logger{
			Filename:   p.Config.Stdout,
			MaxSize:    10, // megabytes
			MaxBackups: 8,
			MaxAge:     1,     //days
			Compress:   false, // disabled by default
		}
		defer w.Close()

		io.WriteString(w, "\r\n----- proc start -----\r\n")
		cmd.Stdout = w
	}

	if p.Config.Stderr != "" {
		if p.Config.Stderr == "&stdout" {
			cmd.Stderr = cmd.Stdout
		} else {
			w := &lumberjack.Logger{
				Filename:   p.Config.Stderr,
				MaxSize:    10, // megabytes
				MaxBackups: 8,
				MaxAge:     1,     //days
				Compress:   false, // disabled by default
			}
			defer w.Close()
			cmd.Stderr = w
		}
	} else {
		cmd.Stderr = cmd.Stdout
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error starting: %v", err)
		return
	}

	// cmd.Wait() may blocked for ever in the win32.
	ch := make(chan error, 1)
	go func() {
		ch <- cmd.Wait()
	}()

	select {
	case <-p.exit:
		// isRunning = false
		cmd.Process.Kill()
	case err, ok := <-ch:
		if err != nil {
			if ok {
				log.Printf("Error running: %v", err)
			}
			io.WriteString(cmd.Stdout, err.Error())
		}
		io.WriteString(cmd.Stdout, "\r\n----- proc end -----\r\n")
	}
}

func (p *Program) Stop(s service.Service) error {
	log.Println("Stopping ", p.Config.DisplayName)
	close(p.exit)
	return nil
}
