package scanner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/lgldsilva/updash/internal/model"
)

// NvmSource checks nvm availability (unix nvm or nvm-windows).
type NvmSource struct{}

func (s *NvmSource) Category() model.Category { return model.CatNvm }
func (s *NvmSource) Label() string            { return binNvm }
func (s *NvmSource) Icon() string             { return "⬡" }

// nvmInstalled reports whether nvm (unix) or nvm-windows is present.
func nvmInstalled(plat model.PlatformInfo) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if isDir(filepath.Join(home, ".nvm")) {
		return true
	}
	if plat.OS == "windows" {
		// nvm-windows installs under %APPDATA%\nvm (or %USERPROFILE%\AppData\Roaming\nvm).
		if appData := os.Getenv("APPDATA"); appData != "" && isDir(filepath.Join(appData, binNvm)) {
			return true
		}
		return isDir(filepath.Join(home, "AppData", "Roaming", binNvm))
	}
	return false
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func (s *NvmSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	if nvmInstalled(plat) {
		return []*model.Item{
			{Name: binNvm, Category: model.CatNvm, Status: model.StatusOK, CurrentVer: statusInstalled},
		}, nil
	}
	return []*model.Item{
		{Name: binNvm, Category: model.CatNvm, Status: model.StatusOK, CurrentVer: "not installed"},
	}, nil
}

// OmzSource checks Oh My Zsh availability.
type OmzSource struct{}

func (s *OmzSource) Category() model.Category { return model.CatOmz }
func (s *OmzSource) Label() string            { return "Oh My Zsh" }
func (s *OmzSource) Icon() string             { return "💻" }

// omzInstalled reports whether ~/.oh-my-zsh exists (any OS with a shell home).
func omzInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return isDir(filepath.Join(home, ".oh-my-zsh"))
}

func (s *OmzSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	if omzInstalled() {
		return []*model.Item{
			{Name: "omz", Category: model.CatOmz, Status: model.StatusOK, CurrentVer: statusInstalled},
		}, nil
	}
	return []*model.Item{
		{Name: "omz", Category: model.CatOmz, Status: model.StatusOK, CurrentVer: "not installed"},
	}, nil
}
