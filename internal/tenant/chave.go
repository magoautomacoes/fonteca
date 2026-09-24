// Package tenant responde "quem e voce e pode continuar": autenticacao por
// chave de API, limite de requisicoes e registro de uso. Nao conhece HTTP
// nem o dominio de CNPJ.
package tenant

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// PrefixoChave marca a chave para que varredura de segredo em repositorio
// (gitleaks e afins) consiga reconhece-la.
const PrefixoChave = "fnt_live_"

// Parametros do Argon2id. Custo deliberado: ~50ms por verificacao.
const (
	argonTempo   uint32 = 3
	argonMemoria uint32 = 64 * 1024 // 64 MiB
	argonThreads uint8  = 4
	argonTamanho uint32 = 32
	argonSalt           = 16
)

// GerarChave sorteia uma chave de 32 bytes e devolve os tres valores:
// a chave em claro (mostrada ao usuario UMA vez e nunca armazenada), o
// lookup deterministico (indice) e o hash Argon2id (autenticacao).
func GerarChave() (chave, lookup, argon string, err error) {
	bruto := make([]byte, 32)
	if _, err = rand.Read(bruto); err != nil {
		return "", "", "", fmt.Errorf("sortear chave: %w", err)
	}
	chave = PrefixoChave + base64.RawURLEncoding.EncodeToString(bruto)

	argon, err = hashArgon(chave)
	if err != nil {
		return "", "", "", err
	}
	return chave, HashLookup(chave), argon, nil
}

// HashLookup e o indice deterministico da chave. SHA-256 puro seria fraco
// para senha de usuario, mas aqui o segredo tem 32 bytes de crypto/rand:
// nao ha dicionario a percorrer. Ele so localiza a linha; quem autentica
// e o Argon2id.
func HashLookup(chave string) string {
	soma := sha256.Sum256([]byte(chave))
	return hex.EncodeToString(soma[:])
}

func hashArgon(chave string) (string, error) {
	salt := make([]byte, argonSalt)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("sortear salt: %w", err)
	}
	soma := argon2.IDKey([]byte(chave), salt, argonTempo, argonMemoria, argonThreads, argonTamanho)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemoria, argonTempo, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(soma)), nil
}

// VerificarArgon confere a chave contra o hash no formato PHC. A comparacao
// e em tempo constante: comparacao ingenua vaza a chave por timing.
func VerificarArgon(chave, hash string) bool {
	partes := strings.Split(hash, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false
	}

	var memoria, tempo uint32
	var threads uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &memoria, &tempo, &threads); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return false
	}
	esperado, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil {
		return false
	}

	// Validar faixa de parametros. Esses valores vem do banco e argon2.IDKey
	// entra em panic com valores degenerados (t<1, p<1). Alem disso, o piso
	// garante que o hash nao seja mais fraco que o que geramos hoje. Os tetos
	// (memoria e tempo) protegem contra consumo descontrolado de recurso em
	// requisicao de borda: aceitamos ate 512 MiB (8x o padrao) e ate 10x o
	// tempo padrao.
	//
	// ATENCAO A QUEM FOR AUMENTAR OS PARAMETROS: o piso compara com as
	// constantes ATUAIS, entao subir argonMemoria/argonTempo/argonThreads faz
	// esta funcao passar a recusar TODA chave ja emitida com os parametros
	// antigos — a autenticacao para ate as chaves serem reemitidas. E uma troca
	// deliberada (impedir downgrade custa rotacao suave); aqui ela e aceitavel
	// porque chave de API se reemite com GerarChave, ao contrario de senha de
	// usuario. Se for endurecer, planeje a migracao junto.
	if tempo < argonTempo || threads < argonThreads || memoria < argonMemoria {
		return false
	}
	if memoria > 512*1024 { // 512 MiB
		return false
	}
	// Teto de tempo, pelo mesmo motivo: t acima de 10x o padrao faria cada
	// verificacao custar segundos de CPU.
	if tempo > 10*argonTempo {
		return false
	}
	if len(salt) == 0 || len(esperado) == 0 {
		return false
	}

	obtido := argon2.IDKey([]byte(chave), salt, tempo, memoria, threads, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(obtido, esperado) == 1
}
