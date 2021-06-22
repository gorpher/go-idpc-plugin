package plugin

import (
	"bytes"
	"os/exec"
	"runtime"
	"testing"
)

func TestParseCommand(t *testing.T) {
	name := "testapp"
	metadata := "metadata"
	outbuf := &bytes.Buffer{}
	errbuf := &bytes.Buffer{}
	cmd := exec.Command("go", "run", "testdata/metadata-plugin.go", "version", "-name", name, "-type", metadata)
	cmd.Stdout = outbuf
	cmd.Stderr = errbuf

	if err := cmd.Run(); err != nil {
		t.Fatal(err)
		return
	}
	if errbuf.Len() != 0 {
		t.Fatal(errbuf.String())
		return
	}
	m := ParseVersionCommand(outbuf.String())
	if m.Key == "" {
		t.Fatal("ParseCommandError")
	}
	if m.GOOS != runtime.GOOS {
		t.Fatal("Parse go_version Error")
	}
	if m.GOVersion != runtime.Version() {
		t.Fatal("Parse go_version Error")
	}
	if m.GOARCH != runtime.GOARCH {
		t.Fatal("Parse goarch Error")
	}
	if string(m.Type) != metadata {
		t.Fatal("Parse type Error")
	}
	if m.Name() != PLUGIN_PREFIX+"-"+name+"-"+metadata {
		t.Fatal("Parse name Error")
	}
}

func TestRunCommandArgs(t *testing.T) {

	args := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := exec.Command("go", "run", "testdata/metadata-plugin.go", "-metadata", `{"go_version":"go1.16.5","goarch":"amd64","goos":"windows","name":"idpc-plugin-metadata","revision":"923088e2","type":"metadata","version":"0.71.2"}`)
	cmd.Stdout = args
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		t.Fatal(err)
		return
	}
	if stderr.Len() != 0 {
		t.Fatal(stderr.String())
		return
	}

	t.Log(args)

	args = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	cmd = exec.Command("go", "run", "testdata/metadata-plugin.go", "version")
	cmd.Stdout = args
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
		return
	}
	if stderr.Len() != 0 {
		t.Fatal(stderr.String())
		return
	}
	t.Log(args)
}

func TestParseVersion(t *testing.T) {
	version, err := ParseVersion("1212.34.1212")
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(version)
}
