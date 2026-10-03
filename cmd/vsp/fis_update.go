package main

// FIS fix-8v: "vsp update" for the FIS fork.
//
//   - UpdateRepo (ldflags -X main.UpdateRepo=owner/repo, or env VSP_UPDATE_REPO)
//     points the update lookup at the fork that built the binary instead of
//     upstream oisee/vibing-steampunk. The FIS release workflow sets it to
//     $GITHUB_REPOSITORY, so a FIS build only ever updates from FIS releases.
//   - FIS release tags are fis-YYYY.MM.DD or fis-YYYY.MM.DD.N (several builds a
//     day). parseVersion reads them as [YYYY, MM*100+DD, N] so they compare in
//     date order; a FIS tag never compares against a semver tag because the
//     two series live on different repositories.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// UpdateRepo is the GitHub owner/repo "vsp update" reads releases from.
var UpdateRepo = ""

var (
	reRepo       = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	reFISVersion = regexp.MustCompile(`^fis-(\d{4})\.(\d{2})\.(\d{2})(?:\.(\d{1,3}))?$`)
)

func init() {
	if r := strings.TrimSpace(os.Getenv("VSP_UPDATE_REPO")); r != "" {
		UpdateRepo = r
	}
	if reRepo.MatchString(UpdateRepo) {
		updateAPIBase = "https://api.github.com/repos/" + UpdateRepo
	}
}

// parseFISVersion reports isFIS for any "fis-" string; ok only for a well-formed tag.
func parseFISVersion(s string) (v [3]int, ok, isFIS bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(s, "fis-") {
		return v, false, false
	}
	m := reFISVersion.FindStringSubmatch(s)
	if m == nil {
		return v, false, true
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return v, false, true
	}
	n := 0
	if m[4] != "" {
		n, _ = strconv.Atoi(m[4])
	}
	return [3]int{y, mo*100 + d, n}, true, true
}
