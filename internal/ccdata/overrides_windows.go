//go:build windows

package ccdata

import "golang.org/x/sys/windows/registry"

// registryOverrides reads the "Settings" policy value (JSON) under
// HKLM\SOFTWARE\Policies\ClaudeCode and HKCU\SOFTWARE\Policies\ClaudeCode.
func registryOverrides() []Override {
	var out []Override
	roots := []struct {
		root registry.Key
		name string
		note string
	}{
		{registry.LOCAL_MACHINE, `HKLM\SOFTWARE\Policies\ClaudeCode`, "System policy (registry): takes precedence over the user value."},
		{registry.CURRENT_USER, `HKCU\SOFTWARE\Policies\ClaudeCode`, "User policy (registry): used only when no admin policy exists; takes precedence over the user value."},
	}
	for _, r := range roots {
		k, err := registry.OpenKey(r.root, `SOFTWARE\Policies\ClaudeCode`, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		val, _, err := k.GetStringValue("Settings")
		k.Close()
		if err != nil {
			continue
		}
		if days, found, err := cleanupDaysFromJSON([]byte(val)); err == nil && found {
			out = append(out, Override{Source: r.name, Kind: "managed", Days: days, Note: r.note})
		}
	}
	return out
}
