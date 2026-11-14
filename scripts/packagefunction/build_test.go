package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBuildRejectsFailedStages(t *testing.T) {
	for _, stage := range []string{"environment command", "environment JSON", "compiler version", "module verify", "compile", "missing executable", "malformed executable"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			calls := 0
			got, err := buildFunction(dir, func(args ...string) ([]byte, error) {
				calls++
				switch calls {
				case 1:
					if stage == "environment command" {
						return nil, errors.New("go failed")
					}
					if stage == "environment JSON" {
						return []byte("{"), nil
					}
					env := environment{GOROOT: dir, GOMODCACHE: dir, GOVERSION: "go1.27.4"}
					if stage == "compiler version" {
						env.GOVERSION = "go1.28.0"
					}
					return json.Marshal(env)
				case 2:
					if !slices.Equal(args, []string{"mod", "verify"}) {
						t.Fatalf("unexpected verification %v", args)
					}
					if stage == "module verify" {
						return nil, errors.New("module modified")
					}
				case 3:
					if stage == "compile" {
						return nil, errors.New("compiler failed")
					}
					if stage == "malformed executable" {
						return nil, os.WriteFile(filepath.Join(dir, "func"), []byte("not ELF"), 0755)
					}
				default:
					t.Fatal("unexpected command")
				}
				return nil, nil
			})
			if err == nil || got != nil {
				t.Fatalf("accepted %s; commands=%d", stage, calls)
			}
		})
	}
}

func TestBuildActualFunctionAndInspectCorruption(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	entries, err := buildFunction(t.TempDir(), executeGo)
	if err != nil {
		t.Fatalf("actual build: %v", err)
	}
	if len(entries) != 12 || entries[0].name != "func" {
		t.Fatalf("unexpected archive entries: %d", len(entries))
	}
	binaryData := entries[0].data
	if err := inspectBinary(binaryData); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func([]byte){
		"missing metadata": func(b []byte) {
			i := bytes.Index(b, []byte("\xff Go buildinf:"))
			if i < 0 {
				t.Fatal("build info marker missing")
			}
			b[i] = 0
		},
		"interpreter": func(b []byte) {
			off := binary.LittleEndian.Uint64(b[32:40])
			binary.LittleEndian.PutUint32(b[off:off+4], 3)
		},
		"architecture": func(b []byte) { binary.LittleEndian.PutUint16(b[18:20], 183) },
		"dynamic segment": func(b []byte) {
			off := binary.LittleEndian.Uint64(b[32:40])
			binary.LittleEndian.PutUint32(b[off:off+4], 2)
		},
		"shared library": func(b []byte) { binary.LittleEndian.PutUint16(b[16:18], 3) },
		"no symbols": func(b []byte) {
			off := binary.LittleEndian.Uint64(b[40:48])
			size := binary.LittleEndian.Uint16(b[58:60])
			count := binary.LittleEndian.Uint16(b[60:62])
			for i := uint16(0); i < count; i++ {
				p := off + uint64(i)*uint64(size)
				if binary.LittleEndian.Uint32(b[p+4:p+8]) == 2 {
					binary.LittleEndian.PutUint32(b[p+4:p+8], 0)
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			copyData := slices.Clone(binaryData)
			change(copyData)
			if inspectBinary(copyData) == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
	if inspectBinary([]byte("not executable")) == nil {
		t.Fatal("accepted text")
	}
}
