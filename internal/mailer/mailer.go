// Package mailer envia correo por SMTP sin autenticacion.
//
// En desarrollo apunta a Mailpit, un servidor SMTP falso que NO entrega nada
// al exterior: captura los mensajes y los muestra en http://localhost:8025.
// Asi se puede probar la verificacion de correo y el restablecimiento de
// contrasena sin enviar correos reales ni contratar un proveedor.
package mailer

import (
	"fmt"
	"net/smtp"
	"strings"
)

type Mailer struct {
	addr string
	from string
}

func New(addr, from string) *Mailer {
	return &Mailer{addr: addr, from: from}
}

func (m *Mailer) Send(to, subject, body string) error {
	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	if err := smtp.SendMail(m.addr, nil, m.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("enviar correo a %s: %w", to, err)
	}
	return nil
}
