package release_deletion

import (
	"fmt"
	"os"
	"syscall"
)

func fileIdentity(path string, info os.FileInfo) string {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	handle, err := syscall.CreateFile(name, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(handle)
	var data syscall.ByHandleFileInformation
	if err = syscall.GetFileInformationByHandle(handle, &data); err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d:%s", data.VolumeSerialNumber, data.FileIndexHigh, data.FileIndexLow, data.CreationTime.HighDateTime, data.CreationTime.LowDateTime, identityMetadata(info))
}
