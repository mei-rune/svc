package svc

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime/debug"
	"sync"
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
	service service.Service

	Config Config

	cmd *exec.Cmd

	lock    sync.Mutex
	restart chan struct{}
	exit    chan struct{}
	wait    *sync.WaitGroup
}

func (p *Program) Start(s service.Service) error {
	var wait *sync.WaitGroup
	err := func() error {
		p.lock.Lock()
		defer p.lock.Unlock()

		if p.exit != nil {
			return errors.New("已经启动了")
		}

		wait = new(sync.WaitGroup)
		p.wait = wait
		p.exit = make(chan struct{})
		if p.restart == nil {
			p.restart = make(chan struct{})
		}
		return nil
	}()
	if err != nil {
		return err
	}

	if p.Config.Update.BaseURL != "" {
		updater, err := autoupdate.NewUpdater(p.Config.Update)
		if err != nil {
			log.Println("启用自动升级功能失败,", err)
			return errors.New("启用自动升级功能失败," + err.Error())
		}

		wait.Add(1)
		go func() {
			defer wait.Done()
			RunUpdate(updater, p.restart, p.exit)
		}()

		log.Println("启用自动升级功能！")
	}

	wait.Add(1)
	go func() {
		defer wait.Done()
		p.run()
	}()
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

		// 清空 p.restart 信号
		select {
		case <-p.restart:
		default:
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

	kill := func() {
		err := cmd.Process.Signal(os.Interrupt)
		if err != nil {
			log.Println("send ctrl+c to kill process fail,", err)
		} else {
			timer := time.NewTimer(10 * time.Minute)
			select {
			case err := <-ch:
				timer.Stop()
				io.WriteString(cmd.Stdout, "Error running: ")
				io.WriteString(cmd.Stdout, err.Error())
				io.WriteString(cmd.Stdout, "\r\n----- proc end -----\r\n")
				return
			case <-timer.C:
				log.Println("send ctrl+c to kill process timeout")
			}
		}

		err = cmd.Process.Kill()
		if err != nil {
			log.Println("kill process fail,", err)
		} else {
			timer := time.NewTimer(10 * time.Minute)
			select {
			case err := <-ch:
				timer.Stop()
				io.WriteString(cmd.Stdout, "Error running: ")
				io.WriteString(cmd.Stdout, err.Error())
				io.WriteString(cmd.Stdout, "\r\n----- proc end -----\r\n")
				log.Println("process is exit!")
			case <-timer.C:
				log.Println("kill process timeout")
			}
		}
	}

	select {
	case <-p.exit:
		log.Println("shutdown service")
		kill()
	case <-p.restart:
		log.Println("restart service")
		kill()
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

	p.lock.Lock()
	defer p.lock.Unlock()

	if p.exit == nil {
		return errors.New("已经停止了")
	}

	close(p.exit)
	p.wait.Wait()

	p.exit = nil
	p.wait = nil
	return nil
}
