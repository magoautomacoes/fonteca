package tenant

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestGerarChaveTemPrefixoEEUnica(t *testing.T) {
	chave1, lookup1, argon1, err := GerarChave()
	if err != nil {
		t.Fatalf("gerar: %v", err)
	}
	if !strings.HasPrefix(chave1, PrefixoChave) {
		t.Errorf("chave %q nao tem prefixo %q", chave1, PrefixoChave)
	}
	if lookup1 == "" || argon1 == "" {
		t.Fatal("lookup e argon nao podem ser vazios")
	}
	if strings.Contains(argon1, chave1) {
		t.Error("o hash argon contem a chave em claro")
	}

	chave2, _, _, err := GerarChave()
	if err != nil {
		t.Fatalf("gerar segunda: %v", err)
	}
	if chave1 == chave2 {
		t.Error("duas chamadas geraram a mesma chave")
	}
}

func TestHashLookupEDeterministico(t *testing.T) {
	chave, lookup, _, err := GerarChave()
	if err != nil {
		t.Fatalf("gerar: %v", err)
	}
	if HashLookup(chave) != lookup {
		t.Error("HashLookup nao reproduziu o lookup devolvido por GerarChave")
	}
	if HashLookup("fnt_live_outra") == lookup {
		t.Error("chaves diferentes produziram o mesmo lookup")
	}
}

// O Argon2id embute salt aleatorio: o mesmo segredo gera hashes diferentes.
// E por isso que a busca precisa do lookup deterministico.
func TestArgonVerificaMasNaoEIgualdade(t *testing.T) {
	chave, _, argon, err := GerarChave()
	if err != nil {
		t.Fatalf("gerar: %v", err)
	}
	if !VerificarArgon(chave, argon) {
		t.Error("a chave correta nao verificou")
	}
	if VerificarArgon("fnt_live_errada", argon) {
		t.Error("uma chave errada verificou")
	}

	_, _, argonDeNovo, err := GerarChave()
	if err != nil {
		t.Fatalf("gerar de novo: %v", err)
	}
	if argon == argonDeNovo {
		t.Error("dois hashes identicos: o salt nao esta aleatorio")
	}
}

func TestVerificarArgonRecusaHashMalformado(t *testing.T) {
	for _, ruim := range []string{"", "nao-e-phc", "$argon2id$v=19$lixo"} {
		if VerificarArgon("fnt_live_x", ruim) {
			t.Errorf("hash malformado %q foi aceito", ruim)
		}
	}
}

// TestVerificarArgonRecusaDegenerado testa defesa contra valores que causariam
// panic em argon2.IDKey ou que representariam parametros fracos. Usa defer
// recover() para capturar panics de forma explícita.
func TestVerificarArgonRecusaDegenerado(t *testing.T) {
	chave := "fnt_live_x"

	// Caso 1: rounds e threads zero — argon2.IDKey entra em panic
	testCaseComPanic(t, chave, "$argon2id$v=19$m=0,t=0,p=0$AAAA$BBBB",
		"rounds/threads zero deve ser recusado sem panic")

	// Caso 2: hash esperado vazio — nil dereference em len(esperado)
	testCaseComPanic(t, chave, "$argon2id$v=19$m=65536,t=3,p=4$AAAA$",
		"esperado vazio deve ser recusado sem panic")

	// Caso 3: salt vazio — nil dereference em len(salt)
	testCaseComPanic(t, chave, "$argon2id$v=19$m=65536,t=3,p=4$$BBBB",
		"salt vazio deve ser recusado sem panic")

	// Caso 4: parametros fracos (piso violado) — aceitar seria reduzir defesa
	if VerificarArgon(chave, "$argon2id$v=19$m=1,t=1,p=1$AAAA$BBBB") {
		t.Error("parametros fracos (m=1,t=1,p=1) nao devem ser aceitos")
	}

	// Caso 5: memoria absurda (teto violado) — protege contra DoS
	testCaseComPanic(t, chave, "$argon2id$v=19$m=99999999,t=3,p=4$AAAA$BBBB",
		"memoria absurda (teto) deve ser recusada sem panic")

	// Caso 6: chave legítima gerada por GerarChave() continua verificando
	chaveLegit, _, argonLegit, err := GerarChave()
	if err != nil {
		t.Fatalf("gerar chave legítima: %v", err)
	}
	if !VerificarArgon(chaveLegit, argonLegit) {
		t.Error("chave legítima gerada por GerarChave() nao verificou apos guardrails")
	}
}

// testCaseComPanic verifica que VerificarArgon retorna false e nao entra em panic.
func testCaseComPanic(t *testing.T, chave, hash, msg string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: panic capturado: %v", msg, r)
		}
	}()
	if VerificarArgon(chave, hash) {
		t.Errorf("%s: foi aceito quando deveria ser recusado", msg)
	}
}

// Teto de tempo: um hash valido, mas com t absurdo, faria cada autenticacao
// custar segundos de CPU. Tem de ser recusado mesmo quando a chave confere.
func TestVerificarArgonRecusaTempoAcimaDoTeto(t *testing.T) {
	chave := PrefixoChave + "teto-de-tempo"
	salt := []byte("0123456789abcdef")
	tempo := 10*argonTempo + 1
	soma := argon2.IDKey([]byte(chave), salt, tempo, argonMemoria, argonThreads, argonTamanho)
	hash := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemoria, tempo, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(soma))

	if VerificarArgon(chave, hash) {
		t.Errorf("t=%d (acima de 10x o padrao) foi aceito", tempo)
	}

	// No teto exato ainda vale: o limite e "acima de 10x".
	tempo = 10 * argonTempo
	soma = argon2.IDKey([]byte(chave), salt, tempo, argonMemoria, argonThreads, argonTamanho)
	hash = fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemoria, tempo, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(soma))
	if !VerificarArgon(chave, hash) {
		t.Errorf("t=%d (no teto) foi recusado", tempo)
	}
}
