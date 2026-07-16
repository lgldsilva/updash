package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestArchiveName(t *testing.T) {
	tests := []struct {
		tag, goos, goarch, want string
	}{
		{"v0.1.8", "darwin", "arm64", "updash_0.1.8_darwin_arm64.tar.gz"},
		{"0.1.8", "linux", "amd64", "updash_0.1.8_linux_amd64.tar.gz"},
		{"v1.0.0", "windows", "amd64", "updash_1.0.0_windows_amd64.zip"},
	}
	for _, tt := range tests {
		if got := archiveName(tt.tag, tt.goos, tt.goarch); got != tt.want {
			t.Errorf("archiveName(%q, %q, %q) = %q, want %q", tt.tag, tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestPickReleaseBinary_prefersNamedBinaryOverReadme(t *testing.T) {
	readme := []byte("# updash\nrelease notes\n")
	// Minimal ELF-ish header so looksLikeExecutable accepts a fallback path.
	elf := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	elf = append(elf, []byte("fake-binary-payload")...)

	got, err := pickReleaseBinary([]archiveMember{
		{name: "README.md", data: readme},
		{name: "LICENSE", data: []byte("MIT")},
		{name: "updash", data: elf},
	})
	if err != nil {
		t.Fatalf("pickReleaseBinary: %v", err)
	}
	if !bytes.Equal(got, elf) {
		t.Fatalf("picked wrong member: %q", got)
	}
}

func TestPickReleaseBinary_fallsBackToMagic(t *testing.T) {
	elf := []byte{0x7f, 'E', 'L', 'F', 0x02}
	got, err := pickReleaseBinary([]archiveMember{
		{name: "README.md", data: []byte("docs")},
		{name: "bin/updash-linux", data: elf},
	})
	if err != nil {
		t.Fatalf("pickReleaseBinary: %v", err)
	}
	if !bytes.Equal(got, elf) {
		t.Fatalf("picked wrong member: %q", got)
	}
}

func TestPickReleaseBinary_docsOnlyErrors(t *testing.T) {
	_, err := pickReleaseBinary([]archiveMember{
		{name: "README.md", data: []byte("docs")},
		{name: "LICENSE", data: []byte("MIT")},
	})
	if err == nil {
		t.Fatal("expected error for docs-only archive")
	}
}

func TestExtractFromTarGz_skipsReadme(t *testing.T) {
	elf := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01}
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for _, m := range []struct {
		name string
		body []byte
	}{
		{"README.md", []byte("# release notes\n")},
		{"updash", elf},
	} {
		hdr := &tar.Header{Name: m.name, Mode: 0o755, Size: int64(len(m.body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(m.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromTarGz(buf.Bytes())
	if err != nil {
		t.Fatalf("extractFromTarGz: %v", err)
	}
	if !bytes.Equal(got, elf) {
		t.Fatalf("got %q, want ELF payload", got)
	}
}

func TestExtractFromZip_skipsReadme(t *testing.T) {
	elf := []byte{'M', 'Z', 0x90, 0x00} // PE/DOS
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range []struct {
		name string
		body []byte
	}{
		{"README.md", []byte("# notes\n")},
		{"updash.exe", elf},
	} {
		w, err := zw.Create(m.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(m.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromZip(buf.Bytes())
	if err != nil {
		t.Fatalf("extractFromZip: %v", err)
	}
	if !bytes.Equal(got, elf) {
		t.Fatalf("got %q, want PE payload", got)
	}
}

func TestLooksLikeExecutable(t *testing.T) {
	if !looksLikeExecutable([]byte{0x7f, 'E', 'L', 'F'}) {
		t.Error("ELF")
	}
	if !looksLikeExecutable([]byte{'M', 'Z', 0, 0}) {
		t.Error("PE")
	}
	if looksLikeExecutable([]byte("# markdown")) {
		t.Error("text should not look like executable")
	}
}
