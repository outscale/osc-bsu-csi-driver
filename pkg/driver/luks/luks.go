package luks

type LuksContext struct {
	Cipher  string
	Hash    string
	KeySize string
}

type LuksService interface {
	IsLuks(devicePath string) bool
	LuksFormat(devicePath string, passphrase string, context LuksContext) error
	CheckLuksPassphrase(devicePath string, passphrase string) bool
	LuksOpen(devicePath string, encryptedDeviceName string, passphrase string) (bool, error)
	IsLuksMapping(devicePath string) (bool, string, error)
	LuksResize(deviceName string, passphrase string) error
	LuksClose(deviceName string) error
}