package desktop

import "errors"

var ErrExternalBrowserLaunched = errors.New("desktop: external browser launched")

var ErrNativeDialogCanceled = errors.New("desktop: native file dialog canceled")
