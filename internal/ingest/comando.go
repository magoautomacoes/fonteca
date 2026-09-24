package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magoautomacoes/fonteca/internal/source"
	"github.com/magoautomacoes/fonteca/internal/store"
)

// Opcoes controla uma execucao de ingestao.
type Opcoes struct {
	// Referencia do lote (AAAA-MM). Vazio significa "o mais recente publicado".
	Referencia string
	// Diretorio onde os ZIPs sao guardados entre execucoes, para permitir retomada.
	Diretorio string
	// DryRun lista o que seria feito sem baixar nem gravar.
	DryRun bool
	// Apenas restringe a estes arquivos (por nome). Vazio = todos os suportados.
	Apenas []string
	// CNAEs restringe a carga de estabelecimentos a estes codigos. Vazio
	// carrega tudo. Medido no lote 2026-09: filtrar pelos quatro CNAEs de
	// um ramo de quatro CNAEs guarda 1,23 milhao de linhas em vez de 63 milhoes.
	CNAEs []string
	// CNAESecundario inclui quem tem o codigo como atividade secundaria, nao
	// so principal — uma construtora que tambem fabrica o que vende, por exemplo.
	CNAESecundario bool
	// Fonte substitui a descoberta automatica. Existe para os testes apontarem
	// a um servidor local: sem isso, todo `go test` bateria no portal da Receita.
	// Em producao fica nil e a fonte e descoberta pelo redirect.
	Fonte *source.Fonte
}

// suportado diz se o A2 sabe carregar este arquivo. Empresas e
// Estabelecimentos vem em dez partes cada, numeradas de 0 a 9.
func suportado(nome string) bool {
	switch {
	case nome == "Municipios.zip", nome == "Simples.zip",
		nome == "Cnaes.zip", nome == "Naturezas.zip":
		return true
	case strings.HasPrefix(nome, "Empresas"):
		return parteNumerada(nome, "Empresas")
	case strings.HasPrefix(nome, "Estabelecimentos"):
		return parteNumerada(nome, "Estabelecimentos")
	}
	return false
}

// parteNumerada confere que o nome e exatamente <prefixo><digito>.zip.
// Checar so o comprimento deixaria passar "EmpresasX.zip", que so falharia
// la dentro do download ou do carregador, com um erro confuso.
func parteNumerada(nome, prefixo string) bool {
	resto := strings.TrimPrefix(nome, prefixo)
	if len(resto) != len("0.zip") || !strings.HasSuffix(resto, ".zip") {
		return false
	}
	return resto[0] >= '0' && resto[0] <= '9'
}

// DiretorioPadrao e onde os ZIPs ficam entre execucoes. NAO pode ser /tmp:
// em muitos sistemas e tmpfs (em RAM) e o systemd-tmpfiles limpa entre
// execucoes, o que anula a retomada — e na VM de destino 7,3 GB em tmpfs
// disputariam memoria com o shared_buffers do Postgres.
const DiretorioPadrao = "/var/lib/fonteca/lotes"

// Executar roda o ciclo de ingestao.
func Executar(ctx context.Context, pool *pgxpool.Pool, opts Opcoes) error {
	if opts.Diretorio == "" {
		opts.Diretorio = DiretorioPadrao
	}

	fonte := opts.Fonte
	if fonte == nil {
		var err error
		fonte, err = source.NovaFonte(ctx, nil)
		if err != nil {
			return fmt.Errorf("descobrir a fonte: %w", err)
		}
		slog.Info("fonte descoberta", "hash", fonte.Hash)
	}

	var err error
	referencia := opts.Referencia
	if referencia == "" {
		referencia, err = fonte.MaisRecente(ctx)
		if err != nil {
			return fmt.Errorf("descobrir o lote mais recente: %w", err)
		}
	}

	filtro := NovoFiltroCNAE(opts.CNAEs, opts.CNAESecundario)
	if filtro != nil {
		slog.Info("carga filtrada por CNAE",
			"codigos", opts.CNAEs, "inclui_secundario", opts.CNAESecundario)
	}

	schema, err := store.NomeDoSchema(referencia)
	if err != nil {
		return err
	}

	// Ja importado? Sair sem fazer nada — permite rodar o cron diariamente.
	vigente, err := store.LoteVigente(ctx, pool)
	if err != nil {
		return err
	}
	if vigente != nil && vigente.Schema == schema {
		slog.Info("lote ja importado, nada a fazer", "lote", referencia)
		return nil
	}

	arquivos, err := fonte.ListarLote(ctx, referencia)
	if err != nil {
		return fmt.Errorf("listar o lote %s: %w", referencia, err)
	}

	desejados := filtrar(arquivos, opts.Apenas)
	ordenarPorPrioridadeDeCarga(desejados)
	if len(desejados) == 0 {
		return fmt.Errorf("nenhum arquivo suportado encontrado no lote %s", referencia)
	}

	if opts.DryRun {
		var total int64
		for _, a := range desejados {
			fmt.Printf("  %-32s %10.2f MB  %s\n",
				a.Nome, float64(a.Tamanho)/(1024*1024), a.Modificado.Format(time.DateOnly))
			total += a.Tamanho
		}
		fmt.Printf("lote %s: %d arquivo(s), %.2f GB\n",
			referencia, len(desejados), float64(total)/(1024*1024*1024))
		return nil
	}

	if err := os.MkdirAll(opts.Diretorio, 0o755); err != nil {
		return fmt.Errorf("criar diretorio de lotes: %w", err)
	}

	// O schema novo nasce limpo: se sobrou de uma tentativa anterior, descarta.
	if vigente == nil || vigente.Schema != schema {
		if err := store.DescartarSchema(ctx, pool, schema); err != nil {
			return fmt.Errorf("limpar tentativa anterior: %w", err)
		}
	}
	if err := store.CriarSchemaDeLote(ctx, pool, schema); err != nil {
		return err
	}

	var linhas int64
	for _, a := range desejados {
		destino := filepath.Join(opts.Diretorio, referencia, a.Nome)
		if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
			return fmt.Errorf("criar pasta do lote: %w", err)
		}

		slog.Info("baixando", "arquivo", a.Nome, "mb", a.Tamanho/(1024*1024))
		if err := fonte.Baixar(ctx, referencia, a, destino); err != nil {
			return err
		}

		n, err := carregar(ctx, pool, schema, a.Nome, destino, filtro)
		if err != nil {
			return err
		}
		slog.Info("carregado", "arquivo", a.Nome, "linhas", n)
		linhas += n
	}

	slog.Info("criando indices", "schema", schema)
	if err := store.CriarIndices(ctx, pool, schema); err != nil {
		return err
	}

	if err := store.TrocarLoteVigente(ctx, pool, schema, referencia, linhas); err != nil {
		return err
	}
	slog.Info("lote vigente", "lote", referencia, "linhas", linhas)

	// Descartar o lote anterior so DEPOIS da troca, e nunca o vigente.
	if vigente != nil && vigente.Schema != schema {
		if err := store.DescartarSchema(ctx, pool, vigente.Schema); err != nil {
			slog.Warn("nao foi possivel descartar o lote antigo",
				"schema", vigente.Schema, "erro", err)
		}
	}
	return nil
}

// filtrar decide quais arquivos do lote entram na carga. Com apenas vazio,
// aceita todo arquivo que suportado() reconhece; caso contrario, restringe
// aos nomes explicitamente pedidos (usado pelos testes e por retomadas
// pontuais).
func filtrar(arquivos []source.Arquivo, apenas []string) []source.Arquivo {
	var quer map[string]bool
	if len(apenas) > 0 {
		quer = make(map[string]bool, len(apenas))
		for _, n := range apenas {
			quer[n] = true
		}
	}

	var out []source.Arquivo
	for _, a := range arquivos {
		if quer != nil {
			if quer[a.Nome] {
				out = append(out, a)
			}
			continue
		}
		if suportado(a.Nome) {
			out = append(out, a)
		}
	}
	return out
}

// prioridadeDeCarga ordena os arquivos por tamanho crescente de risco: os
// menores primeiro, Estabelecimentos por ultimo. E de longe o maior arquivo
// (5,1 GB comprimidos, ~63 milhoes de linhas) — se a carga falhar no meio,
// falhar depois que o resto ja foi carregado deixa menos trabalho para a
// retomada.
func prioridadeDeCarga(nome string) int {
	switch {
	case nome == "Municipios.zip", nome == "Cnaes.zip", nome == "Naturezas.zip":
		return 0
	case nome == "Simples.zip":
		return 1
	case strings.HasPrefix(nome, "Empresas"):
		return 2
	case strings.HasPrefix(nome, "Estabelecimentos"):
		return 3
	default:
		return 4
	}
}

// ordenarPorPrioridadeDeCarga fixa a ordem de carga independente da ordem
// devolvida pela listagem WebDAV (que nao e garantida). Estabelecimentos
// sempre por ultimo.
func ordenarPorPrioridadeDeCarga(arquivos []source.Arquivo) {
	sort.SliceStable(arquivos, func(i, j int) bool {
		return prioridadeDeCarga(arquivos[i].Nome) < prioridadeDeCarga(arquivos[j].Nome)
	})
}

// carregar despacha para o carregador do tipo de arquivo.
func carregar(ctx context.Context, pool *pgxpool.Pool, schema, nome, caminho string, filtro *FiltroCNAE) (int64, error) {
	switch {
	case nome == "Municipios.zip":
		return CarregarMunicipios(ctx, pool, schema, caminho)
	case nome == "Simples.zip":
		return CarregarSimples(ctx, pool, schema, caminho)
	case nome == "Cnaes.zip":
		return CarregarCnaes(ctx, pool, schema, caminho)
	case nome == "Naturezas.zip":
		return CarregarNaturezas(ctx, pool, schema, caminho)
	case strings.HasPrefix(nome, "Empresas"):
		return CarregarEmpresas(ctx, pool, schema, caminho)
	case strings.HasPrefix(nome, "Estabelecimentos"):
		return CarregarEstabelecimentosCom(ctx, pool, schema, caminho, filtro)
	default:
		// Socios fica de fora por decisao (docs/docs/arquitetura.md), nao por falta.
		return 0, fmt.Errorf("arquivo %s nao suportado", nome)
	}
}
