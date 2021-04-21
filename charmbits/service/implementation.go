package service

import (
	"github.com/kardianos/service"
	"github.com/pkg/errors"
	"gopkg.in/tomb.v2"
	"os/exec"
	"strings"
)

// OSServiceParams holds the parameters for
// creating a new service.
type OSServiceParams struct {
	// Name holds the name of the service.
	Name string

	// Description holds the description of the service.
	Description string

	// Exe holds the name of the executable to run.
	Exe string

	// Args holds any arguments to the executable,
	// which should be OK to to pass to the shell
	// without quoting.
	Args []string
}


type srv struct {
	p *program
	t tomb.Tomb
	name string
}

func (s *srv) StopAndRemove() error {
	if e := s.Stop(); e != nil {
		return errors.Wrap(e, "can not stop service")
	}
	if e := s.p.Uninstall(); e != nil {
		return errors.Wrap(e, "can not remove service")
	}
	return nil
}

func (s *srv) Running() bool {
	return s.p.IsRunning()
}

func (s *srv) Stop() error {
	return s.p.Stop(s.p.Service)
}

func (s *srv) Start() error {
	return s.p.Start(s.p.Service)
}

func (s *srv) Install() error {
	if  s.p.IsNotInstalled() {
		return s.p.Install()
	} else {
		return nil
	}
}

type program struct {
	service.Service
	name string
}

func (p *program) Start(s service.Service) error {
	return s.Start()
}

func (p *program) Stop(s service.Service) error {
	return s.Stop()
}

// FIXME [upstream]: p.Status() can not be used due to a bug in service_systemd_linux.go
// > exitCode, out, err := runWithOutput("systemctl", "list-unit-files", "-t", "service", s.Name)
// < exitCode, out, err := runWithOutput("systemctl", "list-unit-files", "-t", "service", s.Name + ".service")

func (p *program) IsNotInstalled() bool {
	err := exec.Command("service", p.name, "status").Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode() == 4
	} else {
		return false
	}
}

func (p *program) IsRunning() bool {
	data, err := exec.Command("service", p.name, "status").Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(data), "running")
}

func SystemLogger(osServiceName string) {
	p := &program{name: osServiceName}
	s, er := service.New(p, &service.Config{
		Name:        osServiceName,
	})
	if er != nil {
		panic(er)
	}

	// Calling this function will enable the system logger.
	if _, er = s.SystemLogger(nil); er != nil {
		panic(er)
	}
}

// NewService is used to create a new service.
// It is defined as a variable so that it can be
// replaced for testing purposes.
var NewService = func(p OSServiceParams) OSService {
	cfg := &service.Config{
		Name:        p.Name,
		DisplayName: p.Description,
		Executable:  p.Exe,
		Arguments:   p.Args,
	}

	var er error
	s := &srv{p: &program{name: p.Name}}
	s.p.Service, er = service.New(s.p, cfg)
	if er != nil {
		panic(er)
	}

	return s
}