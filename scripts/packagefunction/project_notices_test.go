package main

import (
	"os"
	"testing"
)

func TestProjectNoticesAreRequired(t *testing.T) {
	for _, name := range []string{"LICENSE", "NOTICE"} {
		for _, missing := range []bool{true, false} {
			t.Run(name+map[bool]string{true: " missing", false: " empty"}[missing], func(t *testing.T) {
				env := noticeFixture(t)
				var err error
				if missing {
					err = os.Remove(name)
				} else {
					err = os.WriteFile(name, nil, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				if got, err := collectNotices(env); err == nil || got != nil {
					t.Fatal("accepted missing or empty project notice")
				}
			})
		}
	}
}
