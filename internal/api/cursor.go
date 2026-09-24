package api

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// O cursor e opaco para o cliente, mas nao e segredo: carrega so a posicao na
// ordenacao (data_inicio, cnpj), que o proprio resultado ja expoe. Nao ha o
// que proteger, entao base64 de um par legivel basta — assinar exigiria uma
// chave e nao compraria nada.
func codificarCursor(dataInicio *time.Time, cnpj string) string {
	var data string
	if dataInicio != nil {
		data = dataInicio.UTC().Format(time.RFC3339)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(data + "|" + cnpj))
}

func decodificarCursor(s string) (*time.Time, string, error) {
	if s == "" {
		return nil, "", fmt.Errorf("cursor vazio")
	}
	bruto, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, "", fmt.Errorf("cursor invalido")
	}
	partes := strings.SplitN(string(bruto), "|", 2)
	if len(partes) != 2 || partes[1] == "" {
		return nil, "", fmt.Errorf("cursor invalido")
	}
	if partes[0] == "" {
		return nil, partes[1], nil
	}
	data, err := time.Parse(time.RFC3339, partes[0])
	if err != nil {
		return nil, "", fmt.Errorf("cursor invalido")
	}
	return &data, partes[1], nil
}
