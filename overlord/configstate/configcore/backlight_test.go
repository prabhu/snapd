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
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/overlord/configstate/configcore"
	"github.com/snapcore/snapd/release"
)

type backlightSuite struct {
	configcoreSuite
}

var _ = Suite(&backlightSuite{})

func (s *backlightSuite) SetUpTest(c *C) {
	s.configcoreSuite.SetUpTest(c)

	s.systemctlArgs = nil

	dirs.SetRootDir(c.MkDir())
	err := os.MkdirAll(filepath.Join(dirs.GlobalRootDir, "/etc/"), 0755)
	c.Assert(err, IsNil)
}

func (s *backlightSuite) TearDownTest(c *C) {
	dirs.SetRootDir("/")
}

func (s *backlightSuite) TestConfigureBacklightServiceMaskIntegration(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()

	s.systemctlArgs = nil
	err := configcore.Run(&mockConf{
		state: s.state,
		conf: map[string]interface{}{
			"system.disable-backlight-service": true,
		},
	})
	c.Assert(err, IsNil)
	c.Check(s.systemctlArgs, DeepEquals, [][]string{
		{"--root", dirs.GlobalRootDir, "mask", "systemd-backlight@.service"},
	})
}

func (s *backlightSuite) TestConfigureBacklightServiceUnmaskIntegration(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()

	s.systemctlArgs = nil
	err := configcore.Run(&mockConf{
		state: s.state,
		conf: map[string]interface{}{
			"system.disable-backlight-service": false,
		},
	})
	c.Assert(err, IsNil)
	c.Check(s.systemctlArgs, DeepEquals, [][]string{
		{"--root", dirs.GlobalRootDir, "unmask", "systemd-backlight@.service"},
	})
}

func (s *backlightSuite) TestFilesystemOnlyApply(c *C) {
	restorer := release.MockOnClassic(false)
	defer restorer()

	conf := configcore.PlainCoreConfig(map[string]interface{}{
		"system.disable-backlight-service": "true",
	})
	tmpDir := c.MkDir()
	c.Assert(configcore.FilesystemOnlyApply(tmpDir, conf), IsNil)

	c.Check(s.systemctlArgs, DeepEquals, [][]string{
		{"--root", tmpDir, "mask", "systemd-backlight@.service"},
	})
}
