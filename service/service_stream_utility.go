// This file is part of arduino-flasher-cli.
//
// Copyright 2025 ARDUINO SA (http://www.arduino.cc/)
//
// This software is released under the GNU General Public License version 3,
// which covers the main part of arduino-flasher-cli.
// The terms of this license can be found at:
// https://www.gnu.org/licenses/gpl-3.0.en.html
//
// You can be released from the requirements of the above licenses by purchasing
// a commercial license. Buying such a license is mandatory if you want to
// modify or otherwise use the software for commercial activities involving the
// Arduino software without disclosing the source code of your own applications.
// To purchase a commercial license, send an email to license@arduino.cc.

package service

import "sync"

// SynchronizedSender is a sender function with an extra protection for
// concurrent writes, if multiple threads call the Send method they will
// be blocked and serialized.
type SynchronizedSender[T any] struct {
	lock          sync.Mutex
	protectedSend func(T) error
}

// Send the message using the underlyng stream.
func (s *SynchronizedSender[T]) Send(value T) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.protectedSend(value)
	return err
}

// NewSynchronizedSend takes a Send function and wraps it in a SynchronizedSender
func NewSynchronizedSend[T any](send func(T) error) *SynchronizedSender[T] {
	return &SynchronizedSender[T]{
		protectedSend: send,
	}
}
