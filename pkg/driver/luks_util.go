package driver

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/driver/luks"
	"k8s.io/klog/v2"
	k8sExec "k8s.io/utils/exec"
)

func IsLuks(exec k8sExec.Interface, devicePath string) bool {
	return exec.Command("cryptsetup", "isLuks", devicePath).Run() == nil
}

func LuksFormat(exec k8sExec.Interface, devicePath string, passphrase string, context luks.LuksContext) error {
	extraArgs := []string{"-v", "--type=luks2", "--batch-mode"}

	if len(context.Cipher) != 0 {
		extraArgs = append(extraArgs, fmt.Sprintf("--cipher=%v", context.Cipher))
	}
	if len(context.Hash) != 0 {
		extraArgs = append(extraArgs, fmt.Sprintf("--hash=%v", context.Hash))

	}
	if len(context.KeySize) != 0 {
		extraArgs = append(extraArgs, fmt.Sprintf("--key-size=%v", context.KeySize))

	}
	extraArgs = append(extraArgs, "luksFormat", devicePath)

	formatCmd := exec.Command("cryptsetup", extraArgs...)
	passwordReader := strings.NewReader(passphrase)
	formatCmd.SetStdin(passwordReader)

	if out, err := formatCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("err: %v, output: %v", err, out)
	}

	return nil
}

func CheckLuksPassphrase(exec k8sExec.Interface, devicePath string, passphrase string) bool {
	checkPassphraseCmd := exec.Command("cryptsetup", "-v", "--type=luks2", "--batch-mode", "--test-passphrase", "luksOpen", devicePath)
	passwordReader := strings.NewReader(passphrase)
	checkPassphraseCmd.SetStdin(passwordReader)
	if _, err := checkPassphraseCmd.CombinedOutput(); err != nil {
		return false
	}

	return true
}

func LuksOpen(exec Mounter, devicePath string, encryptedDeviceName string, passphrase string) (bool, error) {
	if ok, err := exec.ExistsPath("/dev/mapper/" + encryptedDeviceName); err == nil && ok {
		klog.V(4).Info("luks volume is already open")
		return true, nil
	}
	openCmd := exec.Command("cryptsetup", "-v", "--type=luks2", "--batch-mode", "luksOpen", devicePath, encryptedDeviceName)
	passwordReader := strings.NewReader(passphrase)
	openCmd.SetStdin(passwordReader)
	if out, err := openCmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("err: %v, output: %v", err, out)
	}

	return true, nil
}

func IsLuksMapping(exec k8sExec.Interface, devicePath string) (bool, string, error) {
	if strings.HasPrefix(devicePath, "/dev/mapper") {
		mappingName := filepath.Base(devicePath)
		out, err := exec.Command("cryptsetup", "status", mappingName).CombinedOutput()
		if err != nil {
			return false, "", err
		}

		isLuksMapping := false
		for _, statusLine := range strings.Split(string(out), "\n") {
			if strings.Contains(statusLine, "type:") && strings.Contains(strings.ToLower(statusLine), "luks2") {
				isLuksMapping = true
			}
		}
		return isLuksMapping, mappingName, nil
	}
	return false, "", nil
}

func LuksResize(exec k8sExec.Interface, deviceName string, passphrase string) error {
	cryptsetupArgs := []string{"--batch-mode", "resize", deviceName}

	// Créer la commande cryptsetup
	resizeCmd := exec.Command("cryptsetup", cryptsetupArgs...)

	// Passer la passphrase via stdin
	passwordReader := strings.NewReader(passphrase)
	resizeCmd.SetStdin(passwordReader)

	// Exécuter la commande et gérer les erreurs
	if out, err := resizeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("err: %v, output: %v", err, string(out))
	}

	return nil
}

func LuksClose(mounter Mounter, encryptedDeviceName string) error {
	exists, err := mounter.ExistsPath(fmt.Sprintf("/dev/mapper/%v", encryptedDeviceName))
	if err != nil {
		return err
	}

	if !exists {
		klog.V(4).Infof("device %v is not opened", encryptedDeviceName)
		return nil
	}

	if err = mounter.Command("cryptsetup", "-v", "luksClose", encryptedDeviceName).Run(); err != nil {
		klog.V(4).Info("error while closing luks device", encryptedDeviceName)
		return err
	}

	return nil

}
