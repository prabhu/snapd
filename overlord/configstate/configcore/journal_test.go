// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2020 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package configcore_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/sys"
	"github.com/snapcore/snapd/overlord/configstate/configcore"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/release"
	"github.com/snapcore/snapd/systemd"
)

type journalSuite struct {
	state *state.State

	systemdVersion    string
	systemctlArgs     [][]string
	systemctlRestorer func()

	findGidRestore   func()
	chownPathRestore func()
}

var _ = Suite(&journalSuite{})

func (s *journalSuite) SetUpTest(c *C) {
	s.state = state.New(nil)
	s.systemdVersion = "236"
	s.systemctlRestorer = systemd.MockSystemctl(func(args ...string) ([]byte, error) {
		s.systemctlArgs = append(s.systemctlArgs, args[:])
		output := []byte("systemd " + s.systemdVersion + "\n+XYZ")
		return output, nil
	})
	s.systemctlArgs = nil
	dirs.SetRootDir(c.MkDir())

	err := os.MkdirAll(filepath.Join(dirs.GlobalRootDir, "/etc/"), 0755)
	c.Assert(err, IsNil)

	s.findGidRestore = configcore.MockFindGid(func(group string) (uint64, error) {
		c.Assert(group, Equals, "systemd-journal")
		return 1234, nil
	})

	s.chownPathRestore = configcore.MockChownPath(func(path string, uid sys.UserID, gid sys.GroupID) error {
		c.Check(uid, Equals, sys.UserID(0))
		c.Check(gid, Equals, sys.GroupID(1234))
		return nil
	})
}

func (s *journalSuite) TearDownTest(c *C) {
	s.systemctlRestorer()
	dirs.SetRootDir("/")
	s.findGidRestore()
	s.chownPathRestore()
}

func (s *journalSuite) TestConfigurePersistentJournalInvalid(c *C) {
	err := configcore.Run(&mockConf{
		state: s.state,
		conf:  map[string]interface{}{"journal.persistent": "foo"},
	})
	c.Assert(err, ErrorMatches, `journal.persistent can only be set to 'true' or 'false'`)
}

func (s *journalSuite) TestConfigurePersistentJournalOnCore(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()

	err := configcore.Run(&mockConf{
		state: s.state,
		conf:  map[string]interface{}{"journal.persistent": "true"},
	})
	c.Assert(err, IsNil)

	c.Check(s.systemctlArgs, DeepEquals, [][]string{
		{"--version"},
		{"kill", "systemd-journald", "-s", "USR1", "--kill-who=all"},
	})

	exists, _, err := osutil.DirExists(filepath.Join(dirs.GlobalRootDir, "/var/log/journal"))
	c.Assert(err, IsNil)
	c.Check(exists, Equals, true)
	c.Check(osutil.FileExists(filepath.Join(dirs.GlobalRootDir, "/var/log/journal/.snapd-created")), Equals, true)
}

func (s *journalSuite) TestConfigurePersistentJournalOldSystemd(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()

	s.systemdVersion = "235"

	err := configcore.Run(&mockConf{
		state: s.state,
		conf:  map[string]interface{}{"journal.persistent": "true"},
	})
	c.Assert(err, IsNil)

	c.Check(s.systemctlArgs, DeepEquals, [][]string{
		{"--version"}, // version query, but no usr1 signal sent
	})

	exists, _, err := osutil.DirExists(filepath.Join(dirs.GlobalRootDir, "/var/log/journal"))
	c.Assert(err, IsNil)
	c.Check(exists, Equals, true)
	c.Check(osutil.FileExists(filepath.Join(dirs.GlobalRootDir, "/var/log/journal/.snapd-created")), Equals, true)
}

func (s *journalSuite) TestConfigurePersistentJournalOnCoreNoopIfExists(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()

	// existing journal directory, not created by snapd (no marker file)
	c.Assert(os.MkdirAll(filepath.Join(dirs.GlobalRootDir, "/var/log/journal"), 0755), IsNil)

	err := configcore.Run(&mockConf{
		state: s.state,
		conf:  map[string]interface{}{"journal.persistent": "true"},
	})
	c.Assert(err, IsNil)

	// systemctl was not called
	c.Check(s.systemctlArgs, HasLen, 0)

	exists, _, err := osutil.DirExists(filepath.Join(dirs.GlobalRootDir, "/var/log/journal"))
	c.Assert(err, IsNil)
	c.Check(exists, Equals, true)

	// marker was not created
	c.Check(osutil.FileExists(filepath.Join(dirs.GlobalRootDir, "/var/log/journal/.snapd-created")), Equals, false)
}

func (s *journalSuite) TestDisablePersistentJournalNotManagedBySnapdError(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()

	// journal directory exists, but no marker file
	c.Assert(os.MkdirAll(filepath.Join(dirs.GlobalRootDir, "/var/log/journal"), 0755), IsNil)

	err := configcore.Run(&mockConf{
		state: s.state,
		conf:  map[string]interface{}{"journal.persistent": "false"},
	})
	c.Assert(err, ErrorMatches, `.*/var/log/journal directory was not created by snapd.*`)
	exists, _, _ := osutil.DirExists(filepath.Join(dirs.GlobalRootDir, "/var/log/journal"))
	c.Check(exists, Equals, true)
}

func (s *journalSuite) TestDisablePersistentJournalOnCore(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()

	c.Assert(os.MkdirAll(filepath.Join(dirs.GlobalRootDir, "/var/log/journal"), 0755), IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(dirs.GlobalRootDir, "/var/log/journal/.snapd-created"), nil, 0755), IsNil)

	err := configcore.Run(&mockConf{
		state: s.state,
		conf:  map[string]interface{}{"journal.persistent": "false"},
	})
	c.Assert(err, IsNil)

	c.Check(s.systemctlArgs, DeepEquals, [][]string{
		{"--version"},
		{"kill", "systemd-journald", "-s", "USR1", "--kill-who=all"},
	})

	exists, _, err := osutil.DirExists(filepath.Join(dirs.GlobalRootDir, "/var/log/journal"))
	c.Assert(err, IsNil)
	c.Check(exists, Equals, false)
}

func (s *journalSuite) TestFilesystemOnlyApply(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()

	conf := configcore.PlainCoreConfig(map[string]interface{}{
		"journal.persistent": "true",
	})
	tmpDir := c.MkDir()
	c.Assert(configcore.FilesystemOnlyApply(tmpDir, conf), IsNil)
	c.Check(s.systemctlArgs, HasLen, 0)

	exists, _, err := osutil.DirExists(filepath.Join(tmpDir, "/var/log/journal"))
	c.Assert(err, IsNil)
	c.Check(exists, Equals, true)
}
