package main

import (
	"errors"
	"strings"
)

var (
	ErrInvalidEmail  = errors.New("etl: user email is invalid or empty")
	ErrInvalidName   = errors.New("etl: product name is empty")
	ErrNegativePrice = errors.New("etl: product price cannot be negative")
)

// Validator ensures incoming MariaDB records meet PostgreSQL schema constraints.
type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) ValidateUser(u *SourceUser) error {
	if strings.TrimSpace(u.Email) == "" || !strings.Contains(u.Email, "@") {
		return ErrInvalidEmail
	}
	if u.ID <= 0 {
		return errors.New("etl: user ID must be positive")
	}
	return nil
}

func (v *Validator) ValidateProduct(p *SourceProduct) error {
	if strings.TrimSpace(p.NameAr) == "" && strings.TrimSpace(p.NameEn) == "" {
		return ErrInvalidName
	}
	if p.Price < 0 {
		return ErrNegativePrice
	}
	if p.ID <= 0 {
		return errors.New("etl: product ID must be positive")
	}
	return nil
}
