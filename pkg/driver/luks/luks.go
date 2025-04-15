package luks

type LuksContext struct {
	Cipher  string
	Hash    string
	KeySize string
}

type LuksService interface {
	IsLuks(devicePath string) bool
	LuksFormat(devicePath, passphrase string, context LuksContext) error
	CheckLuksPassphrase(devicePath, passphrase string) error
	LuksOpen(devicePath, encryptedDeviceName, passphrase string, luksOpenFlags ...string) (bool, error)
	IsLuksMapping(devicePath string) (bool, string, error)
	LuksResize(deviceName, passphrase string) error
	LuksClose(deviceName string) error
}
