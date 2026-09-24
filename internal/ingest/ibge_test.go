package ingest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/magoautomacoes/fonteca/internal/store"
)

// Amostra real do TABMUN.csv do Tesouro, com o nome preenchido de espacos.
const amostraTabMun = "0001;05893631000109;GUAJARA-MIRIM                                ;RO;1100106\r\n" +
	"7107;46395000000139;SAO PAULO                                    ;SP;3550308\r\n" +
	"\r\n" +
	"9999;00000000000000;SEM IBGE                                     ;XX;\r\n" + // sem codigo IBGE
	"lixo sem separador\r\n" +
	"8889;87613642000144;SAO PAULO DAS MISSOES                        ;RS;4319307" // sem CRLF final

func TestLerTabMun(t *testing.T) {
	linhas, err := LerTabMun(strings.NewReader(amostraTabMun))
	if err != nil {
		t.Fatalf("LerTabMun: %v", err)
	}
	if len(linhas) != 3 {
		t.Fatalf("%d linhas, quer 3 (branca, sem IBGE e lixo ficam de fora): %+v", len(linhas), linhas)
	}
	sp := linhas[1]
	if sp.CodigoSIAFI != 7107 || sp.CodigoIBGE != 3550308 || sp.Nome != "SAO PAULO" || sp.UF != "SP" {
		t.Errorf("Sao Paulo lido como %+v", sp)
	}
	if linhas[2].CodigoIBGE != 4319307 {
		t.Errorf("ultima linha sem CRLF perdida: %+v", linhas[2])
	}
}

// Arquivo cortado ou pagina de erro nao pode apagar/encolher o de-para.
func TestCarregarIBGERecusaTabelaPequena(t *testing.T) {
	pool := bancoDeTeste(t)
	if _, err := CarregarIBGE(context.Background(), pool, strings.NewReader(amostraTabMun)); err == nil {
		t.Fatal("aceitou tabela com 3 municipios")
	}
}

func TestCarregarIBGEGravaDePara(t *testing.T) {
	ctx := context.Background()
	pool := bancoDeTeste(t)

	var b strings.Builder
	b.WriteString(amostraTabMun + "\r\n")
	for i := 0; i < minimoDeMunicipios; i++ {
		fmt.Fprintf(&b, "%d;00000000000000;MUNICIPIO %d;MG;%d\r\n", 20000+i, i, 3100000+i)
	}

	n, err := CarregarIBGE(ctx, pool, strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("CarregarIBGE: %v", err)
	}
	if n != minimoDeMunicipios+3 {
		t.Errorf("gravou %d, quer %d", n, minimoDeMunicipios+3)
	}

	ibge, err := store.IBGEPorSIAFI(ctx, pool, 7107)
	if err != nil {
		t.Fatalf("IBGEPorSIAFI: %v", err)
	}
	if ibge != 3550308 {
		t.Errorf("SIAFI 7107 -> %d, quer 3550308", ibge)
	}

	// Idempotente: rodar de novo nao duplica nem falha.
	if _, err := CarregarIBGE(ctx, pool, strings.NewReader(b.String())); err != nil {
		t.Fatalf("segunda carga: %v", err)
	}
}
