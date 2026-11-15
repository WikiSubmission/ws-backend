//go:build windows

package desktop

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// asciiFallback first tries the 8.3 short form of abs, which is ASCII when
// short names are enabled on the volume (the default for the system drive).
// Otherwise it uses %ProgramData%\WikiSubmission\Desktop\<user SID>, with an
// ACL that only the current user and SYSTEM can open.
func asciiFallback(abs string) (string, error) {
	if short, err := shortPath(abs); err == nil && isASCII(short) {
		return short, nil
	}

	programData := os.Getenv("ProgramData")
	if programData == "" || !isASCII(programData) {
		programData = `C:\ProgramData`
	}
	sid, err := currentUserSID()
	if err != nil {
		return "", fmt.Errorf("desktop: no ASCII path for %s and no user SID: %w", abs, err)
	}
	dir := filepath.Join(programData, "WikiSubmission", "Desktop", sid.String())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("desktop: create %s: %w", dir, err)
	}
	if err := restrictToUser(dir, sid); err != nil {
		return "", fmt.Errorf("desktop: restrict %s: %w", dir, err)
	}
	return dir, nil
}

func shortPath(p string) (string, error) {
	in, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(in, &buf[0], uint32(len(buf)))
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

func currentUserSID() (*windows.SID, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return tokenUser.User.Sid.Copy()
}

// restrictToUser replaces the folder's DACL with full control for the user and
// SYSTEM only, and stops inheritance from %ProgramData% (which grants read to
// every local user).
func restrictToUser(dir string, user *windows.SID) error {
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	entries := []windows.EXPLICIT_ACCESS{
		explicitFullControl(user, windows.TRUSTEE_IS_USER),
		explicitFullControl(system, windows.TRUSTEE_IS_WELL_KNOWN_GROUP),
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil)
}

func explicitFullControl(sid *windows.SID, kind windows.TRUSTEE_TYPE) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  kind,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
