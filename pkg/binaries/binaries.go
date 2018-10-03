package binaries

import (
	"os/exec"
	"runtime"
)

const EksctlVersion = "0.1.3"

const HeptioAuthenticatorAwsVersion = "1.10.3"

func BinaryWithExtension(binary string) string {
	if runtime.GOOS == "windows" {
		return binary + ".exe"
	}
	return binary
}

func LookupForBinary(binary string) (string, error) {
	path, err := exec.LookPath(BinaryWithExtension(binary))
	if err != nil {
		return "", err
	}

	return path, nil
}

type VersionExtractor interface {
	arguments() []string

	extractVersion(command string, arguments []string) (string, error)
}

func ShouldInstallBinary(binary string, expectedVersion string, versionExtractor VersionExtractor) (bool, error) {
	if versionExtractor == nil {
		return true, nil
	}

	binaryPath, err := LookupForBinary(binary)
	if err != nil {
		return true, err
	}
	if binaryPath != "" {
		currentVersion, err := versionExtractor.extractVersion(binaryPath, versionExtractor.arguments())
		if err != nil {
			return true, err
		}
		if currentVersion == expectedVersion {
			return false, nil
		}

	}
	return true, nil
}
