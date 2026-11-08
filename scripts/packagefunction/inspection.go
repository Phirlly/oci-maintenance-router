package main

import (
	"bytes"
	"debug/buildinfo"
	"debug/elf"
	"fmt"
	"runtime/debug"
)

func inspectBinary(data []byte) error {
	executable, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid ELF executable: %w", err)
	}
	defer executable.Close()
	if executable.Class != elf.ELFCLASS64 || executable.Data != elf.ELFDATA2LSB ||
		executable.Machine != elf.EM_X86_64 || executable.Type != elf.ET_EXEC {
		return fmt.Errorf("expected a static ELF64 Linux amd64 executable")
	}
	for _, program := range executable.Progs {
		if program.Type == elf.PT_INTERP || program.Type == elf.PT_DYNAMIC {
			return fmt.Errorf("executable requires a dynamic loader or library")
		}
	}
	symbols, err := executable.Symbols()
	if err != nil || len(symbols) == 0 {
		return fmt.Errorf("executable must retain symbols for vulnerability analysis")
	}
	info, err := buildinfo.Read(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("missing Go build metadata: %w", err)
	}
	return validateBuild(info)
}

func validateBuild(info *debug.BuildInfo) error {
	if info == nil || info.GoVersion != compilerVersion || info.Path != "oci-maintenance-router/cmd/router" ||
		info.Main.Path != "oci-maintenance-router" || info.Main.Replace != nil {
		return fmt.Errorf("unexpected compiler, module or entrypoint")
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		if _, ok := settings[setting.Key]; ok {
			return fmt.Errorf("duplicate build setting")
		}
		settings[setting.Key] = setting.Value
	}
	for key, value := range map[string]string{
		"GOOS": "linux", "GOARCH": "amd64", "GOAMD64": "v1", "CGO_ENABLED": "0",
		"-trimpath": "true", "-buildmode": "exe", "-compiler": "gc",
	} {
		if settings[key] != value {
			return fmt.Errorf("unexpected build setting: %s", key)
		}
	}
	expected := map[string]dependency{}
	for _, module := range reviewedDependencies() {
		expected[module.Path] = module.dependency
	}
	if len(info.Deps) != len(expected) {
		return fmt.Errorf("linked dependency graph changed; review packaging policy")
	}
	for _, module := range info.Deps {
		if module == nil {
			return fmt.Errorf("missing module metadata")
		}
		pin, ok := expected[module.Path]
		if !ok || module.Version != pin.Version || module.Sum != pin.Sum || module.Replace != nil {
			return fmt.Errorf("unreviewed linked dependency: %s", module.Path)
		}
		delete(expected, module.Path)
	}
	return nil
}
