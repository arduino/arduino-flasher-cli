package flasher

// DownloadProgressCB is a callback to get updates on download progress
type DownloadProgressCB func(curr *DownloadProgress)

// Start sends a "start" DownloadProgress message to the callback function
func (d DownloadProgressCB) Start(url, label string) {
	d(&DownloadProgress{
		Message: &DownloadProgress_Start{
			Start: &DownloadProgressStart{
				Url:   url,
				Label: label,
			},
		},
	})
}

// Update sends an "update" DownloadProgress message to the callback function
func (d DownloadProgressCB) Update(downloaded int64, totalSize int64) {
	d(&DownloadProgress{
		Message: &DownloadProgress_Update{
			Update: &DownloadProgressUpdate{
				Downloaded: downloaded,
				TotalSize:  totalSize,
			},
		},
	})
}

// End sends an "end" DownloadProgress message to the callback function
func (d DownloadProgressCB) End(success bool, message string) {
	d(&DownloadProgress{
		Message: &DownloadProgress_End{
			End: &DownloadProgressEnd{
				Success: success,
				Message: message,
			},
		},
	})
}

// TaskProgressCB is a callback to receive progress messages
type TaskProgressCB func(msg *TaskProgress)
