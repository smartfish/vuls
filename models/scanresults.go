/* Vuls - Vulnerability Scanner
Copyright (C) 2016  Future Corporation , Japan.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package models

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/future-architect/vuls/config"
	"github.com/future-architect/vuls/cwe"
	"github.com/future-architect/vuls/util"
)

// ScanResults is a slide of ScanResult
type ScanResults []ScanResult

// ScanResult has the result of scanned CVE information.
type ScanResult struct {
	JSONVersion      int                    `json:"jsonVersion"`
	Lang             string                 `json:"lang"`
	ServerUUID       string                 `json:"serverUUID"`
	ServerName       string                 `json:"serverName"` // TOML Section key
	Family           string                 `json:"family"`
	Release          string                 `json:"release"`
	Container        Container              `json:"container"`
	Platform         Platform               `json:"platform"`
	IPv4Addrs        []string               `json:"ipv4Addrs,omitempty"` // only global unicast address (https://golang.org/pkg/net/#IP.IsGlobalUnicast)
	IPv6Addrs        []string               `json:"ipv6Addrs,omitempty"` // only global unicast address (https://golang.org/pkg/net/#IP.IsGlobalUnicast)
	ScannedAt        time.Time              `json:"scannedAt"`
	ScannedVersion   string                 `json:"scannedVersion"`
	ScannedRevision  string                 `json:"scannedRevision"`
	ScannedBy        string                 `json:"scannedBy"`
	ReportedAt       time.Time              `json:"reportedAt"`
	ReportedVersion  string                 `json:"reportedVersion"`
	ReportedRevision string                 `json:"reportedRevision"`
	ReportedBy       string                 `json:"reportedBy"`
	ScannedCves      VulnInfos              `json:"scannedCves"`
	RunningKernel    Kernel                 `json:"runningKernel"`
	Packages         Packages               `json:"packages"`
	CweDict          CweDict                `json:"cweDict"`
	Optional         map[string]interface{} `json:",omitempty"`
	SrcPackages      SrcPackages            `json:",omitempty"`
	Errors           []string               `json:"errors"`
	Config           struct {
		Scan   config.Config `json:"scan"`
		Report config.Config `json:"report"`
	} `json:"config"`
}

// CweDict is a dictionary for CWE
type CweDict map[string]CweDictEntry

// Get the name, url, top10URL for the specified cweID, lang
func (c CweDict) Get(cweID, lang string) (name, url, top10Rank, top10URL string) {
	cweNum := strings.TrimPrefix(cweID, "CWE-")
	switch config.Conf.Lang {
	case "ja":
		if dict, ok := c[cweNum]; ok && dict.OwaspTopTen2017 != "" {
			top10Rank = dict.OwaspTopTen2017
			top10URL = cwe.OwaspTopTen2017GitHubURLJa[dict.OwaspTopTen2017]
		}
		if dict, ok := cwe.CweDictJa[cweNum]; ok {
			name = dict.Name
			url = fmt.Sprintf("http://jvndb.jvn.jp/ja/cwe/%s.html", cweID)
		} else {
			if dict, ok := cwe.CweDictEn[cweNum]; ok {
				name = dict.Name
			}
			url = fmt.Sprintf("https://cwe.mitre.org/data/definitions/%s.html", cweID)
		}
	default:
		if dict, ok := c[cweNum]; ok && dict.OwaspTopTen2017 != "" {
			top10Rank = dict.OwaspTopTen2017
			top10URL = cwe.OwaspTopTen2017GitHubURLEn[dict.OwaspTopTen2017]
		}
		url = fmt.Sprintf("https://cwe.mitre.org/data/definitions/%s.html", cweID)
		if dict, ok := cwe.CweDictEn[cweNum]; ok {
			name = dict.Name
		}
	}
	return
}

// CweDictEntry is a entry of CWE
type CweDictEntry struct {
	En              *cwe.Cwe `json:"en,omitempty"`
	Ja              *cwe.Cwe `json:"ja,omitempty"`
	OwaspTopTen2017 string   `json:"owaspTopTen2017"`
}

// Kernel has the Release, version and whether need restart
type Kernel struct {
	Release        string `json:"release"`
	Version        string `json:"version"`
	RebootRequired bool   `json:"rebootRequired"`
}

// FilterByCvssOver is filter function.
func (r ScanResult) FilterByCvssOver(over float64) ScanResult {
	filtered := r.ScannedCves.Find(func(v VulnInfo) bool {
		v2Max := v.MaxCvss2Score()
		v3Max := v.MaxCvss3Score()
		max := v2Max.Value.Score
		if max < v3Max.Value.Score {
			max = v3Max.Value.Score
		}
		if over <= max {
			return true
		}
		return false
	})
	r.ScannedCves = filtered
	return r
}

// FilterIgnoreCves is filter function.
func (r ScanResult) FilterIgnoreCves() ScanResult {

	ignoreCves := []string{}
	if len(r.Container.Name) == 0 {
		ignoreCves = config.Conf.Servers[r.ServerName].IgnoreCves
	} else {
		if s, ok := config.Conf.Servers[r.ServerName]; ok {
			if con, ok := s.Containers[r.Container.Name]; ok {
				ignoreCves = con.IgnoreCves
			} else {
				util.Log.Errorf("%s is not found in config.toml",
					r.Container.Name)
				return r
			}
		} else {
			util.Log.Errorf("%s is not found in config.toml",
				r.ServerName)
			return r
		}
	}

	filtered := r.ScannedCves.Find(func(v VulnInfo) bool {
		for _, c := range ignoreCves {
			if v.CveID == c {
				return false
			}
		}
		return true
	})
	r.ScannedCves = filtered
	return r
}

// FilterUnfixed is filter function.
func (r ScanResult) FilterUnfixed() ScanResult {
	if !config.Conf.IgnoreUnfixed {
		return r
	}
	filtered := r.ScannedCves.Find(func(v VulnInfo) bool {
		NotFixedAll := true
		for _, p := range v.AffectedPackages {
			NotFixedAll = NotFixedAll && p.NotFixedYet
		}
		return !NotFixedAll
	})
	r.ScannedCves = filtered
	return r
}

// FilterIgnorePkgs is filter function.
func (r ScanResult) FilterIgnorePkgs() ScanResult {
	ignorePkgsRegexps := []string{}
	if len(r.Container.Name) == 0 {
		ignorePkgsRegexps = config.Conf.Servers[r.ServerName].IgnorePkgsRegexp
	} else {
		if s, ok := config.Conf.Servers[r.ServerName]; ok {
			if con, ok := s.Containers[r.Container.Name]; ok {
				ignorePkgsRegexps = con.IgnorePkgsRegexp
			} else {
				util.Log.Errorf("%s is not found in config.toml",
					r.Container.Name)
				return r
			}
		} else {
			util.Log.Errorf("%s is not found in config.toml",
				r.ServerName)
			return r
		}
	}

	regexps := []*regexp.Regexp{}
	for _, pkgRegexp := range ignorePkgsRegexps {
		re, err := regexp.Compile(pkgRegexp)
		if err != nil {
			util.Log.Errorf("Faild to parse %s, %s", pkgRegexp, err)
			continue
		} else {
			regexps = append(regexps, re)
		}
	}
	if len(regexps) == 0 {
		return r
	}

	filtered := r.ScannedCves.Find(func(v VulnInfo) bool {
		if len(v.AffectedPackages) == 0 {
			return true
		}
		for _, p := range v.AffectedPackages {
			match := false
			for _, re := range regexps {
				if re.MatchString(p.Name) {
					match = true
				}
			}
			if !match {
				return true
			}
		}
		return false
	})

	r.ScannedCves = filtered
	return r
}

// ReportFileName returns the filename on localhost without extention
func (r ScanResult) ReportFileName() (name string) {
	if len(r.Container.ContainerID) == 0 {
		return fmt.Sprintf("%s", r.ServerName)
	}
	return fmt.Sprintf("%s@%s", r.Container.Name, r.ServerName)
}

// ReportKeyName returns the name of key on S3, Azure-Blob without extention
func (r ScanResult) ReportKeyName() (name string) {
	timestr := r.ScannedAt.Format(time.RFC3339)
	if len(r.Container.ContainerID) == 0 {
		return fmt.Sprintf("%s/%s", timestr, r.ServerName)
	}
	return fmt.Sprintf("%s/%s@%s", timestr, r.Container.Name, r.ServerName)
}

// ServerInfo returns server name one line
func (r ScanResult) ServerInfo() string {
	if len(r.Container.ContainerID) == 0 {
		return fmt.Sprintf("%s (%s%s)",
			r.FormatServerName(), r.Family, r.Release)
	}
	return fmt.Sprintf(
		"%s (%s%s) on %s",
		r.FormatServerName(),
		r.Family,
		r.Release,
		r.ServerName,
	)
}

// ServerInfoTui returns server information for TUI sidebar
func (r ScanResult) ServerInfoTui() string {
	if len(r.Container.ContainerID) == 0 {
		line := fmt.Sprintf("%s (%s%s)",
			r.ServerName, r.Family, r.Release)
		if r.RunningKernel.RebootRequired {
			return "[Reboot] " + line
		}
		return line
	}

	fmtstr := "|-- %s (%s%s)"
	if r.RunningKernel.RebootRequired {
		fmtstr = "|-- [Reboot] %s (%s%s)"
	}
	return fmt.Sprintf(fmtstr, r.Container.Name, r.Family, r.Release)
}

// FormatServerName returns server and container name
func (r ScanResult) FormatServerName() (name string) {
	if len(r.Container.ContainerID) == 0 {
		name = r.ServerName
	} else {
		name = fmt.Sprintf("%s@%s",
			r.Container.Name, r.ServerName)
	}
	if r.RunningKernel.RebootRequired {
		name = "[Reboot Required] " + name
	}
	return
}

// FormatTextReportHeadedr returns header of text report
func (r ScanResult) FormatTextReportHeadedr() string {
	var buf bytes.Buffer
	for i := 0; i < len(r.ServerInfo()); i++ {
		buf.WriteString("=")
	}

	return fmt.Sprintf("%s\n%s\n%s, %s, %s\n",
		r.ServerInfo(),
		buf.String(),
		r.ScannedCves.FormatCveSummary(),
		r.ScannedCves.FormatFixedStatus(r.Packages),
		r.FormatUpdatablePacksSummary(),
	)
}

// FormatUpdatablePacksSummary returns a summary of updatable packages
func (r ScanResult) FormatUpdatablePacksSummary() string {
	if !r.isDisplayUpdatableNum() {
		return fmt.Sprintf("%d installed", len(r.Packages))
	}

	nUpdatable := 0
	for _, p := range r.Packages {
		if p.NewVersion == "" {
			continue
		}
		if p.Version != p.NewVersion || p.Release != p.NewRelease {
			nUpdatable++
		}
	}
	return fmt.Sprintf("%d installed, %d updatable",
		len(r.Packages),
		nUpdatable)
}

func (r ScanResult) isDisplayUpdatableNum() bool {
	var mode config.ScanMode
	s, _ := config.Conf.Servers[r.ServerName]
	mode = s.Mode

	if mode.IsOffline() {
		return false
	}
	if mode.IsFastRoot() || mode.IsDeep() {
		return true
	}
	if mode.IsFast() {
		switch r.Family {
		case config.RedHat,
			config.Oracle,
			config.Debian,
			config.Ubuntu,
			config.Raspbian:
			return false
		default:
			return true
		}
	}
	return false
}

// IsContainer returns whether this ServerInfo is about container
func (r ScanResult) IsContainer() bool {
	return 0 < len(r.Container.ContainerID)
}

// IsDeepScanMode checks if the scan mode is deep scan mode.
func (r ScanResult) IsDeepScanMode() bool {
	for _, s := range r.Config.Scan.Servers {
		for _, m := range s.ScanMode {
			if m == "deep" {
				return true
			}
		}
	}
	return false
}

// Container has Container information
type Container struct {
	ContainerID string `json:"containerID"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
}

// Platform has platform information
type Platform struct {
	Name       string `json:"name"` // aws or azure or gcp or other...
	InstanceID string `json:"instanceID"`
}
