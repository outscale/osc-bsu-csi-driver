package driver

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/driver/luks"
	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/driver/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ValidStatus = `/dev/mapper/fake_crypt is active and is in use.
	type:    LUKS2
	cipher:  aes-xts-plain64
	keysize: 512 bits
	key location: dm-crypt
	device:  /dev/fake
	sector size:  512
	offset:  4096 sectors
	size:    234369024 sectors
	mode:    read/write`
)

func TestIsLuks(t *testing.T) {
	mockCtl := gomock.NewController(t)
	devicePath := "/dev/fake"
	// Check Isluks when device is not luks
	mockCommand := mocks.NewMockInterface(mockCtl)
	mockRun := mocks.NewMockCmd(mockCtl)
	mockRun.EXPECT().Run().Return(fmt.Errorf("error"))
	mockCommand.EXPECT().Command(gomock.Eq("cryptsetup"), gomock.Eq("isLuks"), gomock.Eq(devicePath)).Return(mockRun)
	assert.False(t, IsLuks(mockCommand, devicePath))

	// Check when it is luks device
	mockCommand = mocks.NewMockInterface(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)
	mockCommand.EXPECT().Command(gomock.Eq("cryptsetup"), gomock.Eq("isLuks"), gomock.Eq(devicePath)).Return(mockRun)
	mockRun.EXPECT().Run().Return(nil)
	assert.True(t, IsLuks(mockCommand, devicePath))
}

func TestLuksFormat(t *testing.T) {
	mockCtl := gomock.NewController(t)
	devicePath := "/dev/fake"
	passphrase := "thisIsSecret"
	context := luks.LuksContext{
		Cipher:  "",
		Hash:    "",
		KeySize: "",
	}

	// check Format with no parameters
	mockCommand := mocks.NewMockInterface(mockCtl)
	mockRun := mocks.NewMockCmd(mockCtl)
	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("--type=luks2"),
		gomock.Eq("--batch-mode"),
		gomock.Eq("luksFormat"),
		gomock.Eq(devicePath),
	).Return(mockRun)
	mockRun.EXPECT().CombinedOutput().Return([]byte{}, nil)
	mockRun.EXPECT().SetStdin(gomock.Any()).Return()
	require.NoError(t, LuksFormat(mockCommand, devicePath, passphrase, context))

	// Check luksformat with Cipher
	context.Cipher = "OneContext"
	mockCommand = mocks.NewMockInterface(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)
	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("--type=luks2"),
		gomock.Eq("--batch-mode"),
		gomock.Eq(fmt.Sprintf("--cipher=%v", context.Cipher)),
		gomock.Eq("luksFormat"),
		gomock.Eq(devicePath),
	).Return(mockRun)
	mockRun.EXPECT().CombinedOutput().Return([]byte{}, nil)
	mockRun.EXPECT().SetStdin(gomock.Any()).Return()
	require.NoError(t, LuksFormat(mockCommand, devicePath, passphrase, context))

	// Check luksformat with Cipher and Hash
	context.Cipher = "OneContext"
	context.Hash = "OneHash"
	mockCommand = mocks.NewMockInterface(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)
	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("--type=luks2"),
		gomock.Eq("--batch-mode"),
		gomock.Eq(fmt.Sprintf("--cipher=%v", context.Cipher)),
		gomock.Eq(fmt.Sprintf("--hash=%v", context.Hash)),
		gomock.Eq("luksFormat"),
		gomock.Eq(devicePath),
	).Return(mockRun)
	mockRun.EXPECT().CombinedOutput().Return([]byte{}, nil)
	mockRun.EXPECT().SetStdin(gomock.Any()).Return()
	require.NoError(t, LuksFormat(mockCommand, devicePath, passphrase, context))

	// Check luksformat with Cipher, Hash and KeySize
	context.Cipher = "OneContext"
	context.Hash = "OneHash"
	context.KeySize = "KeySize"
	mockCommand = mocks.NewMockInterface(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)
	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("--type=luks2"),
		gomock.Eq("--batch-mode"),
		gomock.Eq(fmt.Sprintf("--cipher=%v", context.Cipher)),
		gomock.Eq(fmt.Sprintf("--hash=%v", context.Hash)),
		gomock.Eq(fmt.Sprintf("--key-size=%v", context.KeySize)),
		gomock.Eq("luksFormat"),
		gomock.Eq(devicePath),
	).Return(mockRun)
	mockRun.EXPECT().CombinedOutput().Return([]byte{}, nil)
	mockRun.EXPECT().SetStdin(gomock.Any()).Return()
	require.NoError(t, LuksFormat(mockCommand, devicePath, passphrase, context))
}

func TestCheckLuksPassphrase(t *testing.T) {
	mockCtl := gomock.NewController(t)
	devicePath := "/dev/fake"
	passphrase := "ThisIsASecret"
	// Check when the passphrase is OK
	mockCommand := mocks.NewMockInterface(mockCtl)
	mockRun := mocks.NewMockCmd(mockCtl)
	mockRun.EXPECT().CombinedOutput().Return([]byte{}, nil)
	mockRun.EXPECT().SetStdin(gomock.Any()).Return()
	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("--type=luks2"),
		gomock.Eq("--batch-mode"),
		gomock.Eq("--test-passphrase"),
		gomock.Eq("luksOpen"),
		gomock.Eq(devicePath),
	).Return(mockRun)

	assert.True(t, CheckLuksPassphrase(mockCommand, devicePath, passphrase))

	// Check when it is luks device
	mockCommand = mocks.NewMockInterface(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)
	mockRun.EXPECT().SetStdin(gomock.Any()).Return()
	mockRun.EXPECT().CombinedOutput().Return([]byte{}, fmt.Errorf("error"))
	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("--type=luks2"),
		gomock.Eq("--batch-mode"),
		gomock.Eq("--test-passphrase"),
		gomock.Eq("luksOpen"),
		gomock.Eq(devicePath),
	).Return(mockRun)

	assert.False(t, CheckLuksPassphrase(mockCommand, devicePath, passphrase))
}

func TestLuksOpen(t *testing.T) {
	mockCtl := gomock.NewController(t)
	devicePath := "/dev/fake"
	passphrase := "ThisIsASecret"

	// Check when normal Open
	mockStat := mocks.NewMockMounter(mockCtl)
	mockRun := mocks.NewMockCmd(mockCtl)
	mockStat.EXPECT().ExistsPath("/dev/mapper/fake_crypt").Return(false, nil)
	mockStat.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("--type=luks2"),
		gomock.Eq("--batch-mode"),
		gomock.Eq("luksOpen"),
		gomock.Eq(devicePath),
		gomock.Eq("fake_crypt"),
	).Return(mockRun)
	mockRun.EXPECT().SetStdin(gomock.Any()).Return()
	mockRun.EXPECT().CombinedOutput().Return([]byte{}, nil)
	ok, err := LuksOpen(mockStat, devicePath, "fake_crypt", passphrase)
	require.NoError(t, err)
	assert.True(t, ok)

	// Check when already opened (idempotency)
	mockStat = mocks.NewMockMounter(mockCtl)
	mockStat.EXPECT().ExistsPath("/dev/mapper/fake_crypt").Return(true, nil)
	ok, err = LuksOpen(mockStat, devicePath, "fake_crypt", passphrase)
	require.NoError(t, err)
	assert.True(t, ok)

	// Check when open failed
	mockStat = mocks.NewMockMounter(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)
	mockStat.EXPECT().ExistsPath("/dev/mapper/fake_crypt").Return(false, nil)
	mockStat.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("--type=luks2"),
		gomock.Eq("--batch-mode"),
		gomock.Eq("luksOpen"),
		gomock.Eq(devicePath),
		gomock.Eq("fake_crypt"),
	).Return(mockRun)
	mockRun.EXPECT().SetStdin(gomock.Any()).Return()
	mockRun.EXPECT().CombinedOutput().Return([]byte{}, fmt.Errorf("error"))
	ok, err = LuksOpen(mockStat, devicePath, "fake_crypt", passphrase)
	require.Error(t, err)
	assert.False(t, ok)
}

func TestIsLuksMapping(t *testing.T) {
	mockCtl := gomock.NewController(t)

	// Check when it is a luks mapping
	mockCommand := mocks.NewMockInterface(mockCtl)
	mockRun := mocks.NewMockCmd(mockCtl)
	devicePath := "/dev/mapper/fake_crypt"

	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("status"),
		gomock.Eq("fake_crypt"),
	).Return(mockRun)

	mockRun.EXPECT().CombinedOutput().Return([]byte(ValidStatus), nil)
	ok, mappingName, err := IsLuksMapping(mockCommand, devicePath)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "fake_crypt", mappingName)

	// Check when it is not a luks mapping
	mockCommand = mocks.NewMockInterface(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)
	devicePath = "/dev/mapper/fake_crypt"

	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("status"),
		gomock.Eq("fake_crypt"),
	).Return(mockRun)

	mockRun.EXPECT().CombinedOutput().Return([]byte("/dev/mapper/fake_crypt is inactive"), nil)
	ok, mappingName, err = IsLuksMapping(mockCommand, devicePath)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "fake_crypt", mappingName)

	// Check when it is not a luks mapping because the device is not a mapping at all
	mockCommand = mocks.NewMockInterface(mockCtl)
	devicePath = "/dev/notmapper/fake_crypt"

	ok, mappingName, err = IsLuksMapping(mockCommand, devicePath)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "", mappingName)
}

func TestLuksResize(t *testing.T) {
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	devicePath := "fake_crypt"
	passphrase := "fake_passphrase"

	// Check normal success
	mockCommand := mocks.NewMockInterface(mockCtl)
	mockRun := mocks.NewMockCmd(mockCtl)

	// Expect Command to be called with only four arguments, as passphrase is passed via stdin
	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("--batch-mode"),
		gomock.Eq("resize"),
		gomock.Eq(devicePath),
	).Return(mockRun)

	// Expect SetStdin to be called with passphrase
	mockRun.EXPECT().SetStdin(gomock.Any()).DoAndReturn(func(r io.Reader) {
		// Check that the reader contains the passphrase
		buf := new(strings.Builder)
		io.Copy(buf, r)
		assert.Equal(t, passphrase, buf.String())
	})

	// Expect CombinedOutput instead of Run
	mockRun.EXPECT().CombinedOutput().Return([]byte(""), nil)

	// Call LuksResize with the mock command and passphrase
	require.NoError(t, LuksResize(mockCommand, devicePath, passphrase))

	// Check failure case
	mockCommand = mocks.NewMockInterface(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)

	mockCommand.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("--batch-mode"),
		gomock.Eq("resize"),
		gomock.Eq(devicePath),
	).Return(mockRun)

	mockRun.EXPECT().SetStdin(gomock.Any()).DoAndReturn(func(r io.Reader) {
		buf := new(strings.Builder)
		io.Copy(buf, r)
		assert.Equal(t, passphrase, buf.String())
	})

	// Expect CombinedOutput to return an error
	mockRun.EXPECT().CombinedOutput().Return([]byte(""), fmt.Errorf("Error"))

	require.Error(t, LuksResize(mockCommand, devicePath, passphrase))
}

func TestLuksClose(t *testing.T) {
	mockCtl := gomock.NewController(t)

	// Check when normal Open
	mockStat := mocks.NewMockMounter(mockCtl)
	mockRun := mocks.NewMockCmd(mockCtl)
	mockStat.EXPECT().ExistsPath("/dev/mapper/fake_crypt").Return(true, nil)
	mockStat.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("luksClose"),
		gomock.Eq("fake_crypt"),
	).Return(mockRun)
	mockRun.EXPECT().Run().Return(nil)
	err := LuksClose(mockStat, "fake_crypt")
	require.NoError(t, err)

	// Check when not opened (idempotency)
	mockStat = mocks.NewMockMounter(mockCtl)
	mockStat.EXPECT().ExistsPath("/dev/mapper/fake_crypt").Return(false, nil)
	err = LuksClose(mockStat, "fake_crypt")
	require.NoError(t, err)

	// Check when open failed
	mockStat = mocks.NewMockMounter(mockCtl)
	mockRun = mocks.NewMockCmd(mockCtl)
	mockStat.EXPECT().ExistsPath("/dev/mapper/fake_crypt").Return(true, nil)
	mockStat.EXPECT().Command(
		gomock.Eq("cryptsetup"),
		gomock.Eq("-v"),
		gomock.Eq("luksClose"),
		gomock.Eq("fake_crypt"),
	).Return(mockRun)
	mockRun.EXPECT().Run().Return(fmt.Errorf("error"))
	err = LuksClose(mockStat, "fake_crypt")
	require.Error(t, err)
}
