// Package service allows this executable to install and remove itself and an
// associated user account on a systemd-based Linux distro.
//
// This package is heavy on convention and light on options. Config.Name is used
// to derive the service name, the home directory, and the service filename.
//
// A systemd service file is embedded in the executable and installed in the
// appropriate place. The file is not yet templated, so changes to Config.Name
// and Config.ExeName in main.go need to be reflected in the service definition.
package service

import (
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

type Config struct {
	Name        string
	Description string
	ExeName     string
}

func (c *Config) userName() string    { return fmt.Sprintf("svc-%s", c.Name) }
func (c *Config) exeFile() string     { return fmt.Sprintf("/svc/%s/%s", c.Name, c.ExeName) }
func (c *Config) systemdFile() string { return fmt.Sprintf("/etc/systemd/system/%s.service", c.Name) }
func (c *Config) configFile() string  { return fmt.Sprintf("/etc/%s/config", c.Name) }
func (c *Config) secretsFile() string { return fmt.Sprintf("/etc/%s/secrets", c.Name) }

//go:embed icbm.service
var serviceDef []byte

//go:embed etc-icbm-config
var defaultConfig []byte

//go:embed etc-icbm-secrets
var defaultSecrets []byte

// Add a system user and identically-named group.
func addSystemUser(userName, homeDir, comment string) (uid, gid int, err error) {
	cmd := exec.Command(
		"useradd",
		"--create-home",
		"--home-dir", homeDir,
		"--system",
		"--shell", "/bin/sh",
		"--user-group",
		"--comment", comment,
		userName,
	)
	if _, err = cmd.Output(); err != nil {
		return
	}
	if uid, gid, err = UidGid(userName); err != nil {
		err = fmt.Errorf("could not look up newly-created user %s: %w", userName, err)
		return
	}
	return
}

// shell executes each line in cmds, stopping on the first error if any.
func shell(cmds ...string) error {
	for _, line := range cmds {
		args := strings.Fields(line)
		if len(args) == 0 {
			continue
		}
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("could not execute %s (%s): %w", line, out, err)
		}
	}
	return nil
}

// WriteIfMissing only creates the file if it doesn't exist.
func WriteIfMissing(filename string, data []byte, perm fs.FileMode) {
	_, err := os.Stat(filename)
	if err == nil {
		// it already exists
		return
	}
	os.WriteFile(filename, data, perm)
}

// Install this running executable to the $HOME/bin directory, using our
// versioning scheme.
func (c *Config) Install() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("Install must be done as root")
	}
	os.Mkdir("/svc", 0755)
	// Create the user if it doesn't exist.
	var uid, gid int
	if _, err := user.Lookup(c.userName()); err != nil {
		uid, gid, err = addSystemUser(c.userName(), "/svc/"+c.Name, c.Description)
		if err != nil {
			return fmt.Errorf("could not create user '%s': %w", c.userName(), err)
		}
		os.Mkdir("/svc/"+c.Name+"/bin", 0644)
		os.Chown("/svc/"+c.Name+"/bin", uid, gid)
	}
	self, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return fmt.Errorf("could not locate ourselves on the filesystem: %w", err)
	}
	err = os.Chown(self, uid, gid) // Give ourself away to the new user.
	if err != nil {
		return fmt.Errorf("could not change exe owner to %s: %w", c.userName(), err)
	}
	err = os.Rename(self, c.exeFile()) // Move ourself into place.
	if err != nil {
		return fmt.Errorf("could not install executable to home dir: %w", err)
	}
	c.StopService()
	// Always overwrite the systemd service definition to ensure it's up to date.
	err = os.WriteFile(c.systemdFile(), serviceDef, 0644)
	if err != nil {
		return fmt.Errorf("could not create systemd service file: %w", err)
	}
	// Only write the default /etc/icbm/* files if they don't exist, as these are under user control.
	WriteIfMissing(c.configFile(), defaultConfig, 0644)
	WriteIfMissing(c.secretsFile(), defaultSecrets, 0600)
	err = shell(
		"systemctl daemon-reload",
		"systemctl enable icbm",
		"systemctl start icbm",
	)
	if err != nil {
		return fmt.Errorf("could not execute command: %w", err)
	}
	return nil
}

func (c *Config) Restart() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("restart must be done as root")
	}
	return shell("systemctl restart " + c.Name)
}

func (c *Config) StopService() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("stop must be done as root")
	}
	return shell(
		"systemctl stop " + c.Name,
	)
}

func (c *Config) Uninstall() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("uninstall must be done as root")
	}
	err := shell(
		"systemctl stop "+c.Name,
		"systemctl disable "+c.Name,
		"sleep 1",
		"userdel "+c.Name,
	)
	return err
}

// UidGid returns the associated ids for the user name.
func UidGid(name string) (uid, gid int, err error) {
	svcUser, err := user.Lookup(name)
	fmt.Sscan(svcUser.Uid, &uid)
	fmt.Sscan(svcUser.Gid, &gid)
	return uid, gid, err
}
