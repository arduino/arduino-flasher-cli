// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

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
