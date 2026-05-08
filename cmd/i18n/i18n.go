package i18n

import "fmt"

type Locale interface {
	Get(msg string, args ...interface{}) string
}

type nullLocale struct{}

func (n nullLocale) Parse([]byte) {}

func (n nullLocale) Get(msg string, args ...interface{}) string {
	return fmt.Sprintf(msg, args...)
}

var locale Locale = &nullLocale{}

func SetLocale(l Locale) {
	locale = l
}

// Tr returns msg translated to the selected locale
// the msg argument must be a literal string
func Tr(msg string, args ...interface{}) string {
	return locale.Get(msg, args...)
}
