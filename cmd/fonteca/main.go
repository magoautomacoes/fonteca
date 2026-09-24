// Command fonteca indexa e consulta fontes de dados publicos brasileiros.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/magoautomacoes/fonteca/internal/api"
	"github.com/magoautomacoes/fonteca/internal/export"
	"github.com/magoautomacoes/fonteca/internal/ingest"
	"github.com/magoautomacoes/fonteca/internal/store"
	"github.com/magoautomacoes/fonteca/internal/tenant"
)

const uso = `fonteca - indexacao de fontes de dados publicos

Uso:
  fonteca <comando> [opcoes]

Comandos:
  migrate   Aplica o schema permanente (meta)
  ingest    Baixa e carrega o lote mais recente
  export    Exporta resultado de busca em CSV
  serve     Sobe a API HTTP
  conta     Cria uma conta e emite a primeira chave de API
  ibge      Carrega o de-para SIAFI -> IBGE do Tesouro Transparente
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, uso)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "migrate":
		err = executarMigrate()
	case "ingest":
		err = executarIngest(os.Args[2:])
	case "export":
		err = executarExport(os.Args[2:])
	case "serve":
		err = executarServe(os.Args[2:])
	case "conta":
		err = executarConta(os.Args[2:])
	case "ibge":
		err = executarIBGE(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(uso)
		return
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n\n%s", os.Args[1], uso)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

func executarMigrate() error {
	dsn := os.Getenv("FONTECA_DSN")
	if dsn == "" {
		return fmt.Errorf("FONTECA_DSN nao definida")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := store.Conectar(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := store.MigrarMeta(ctx, pool); err != nil {
		return err
	}
	fmt.Println("schema meta aplicado")
	return nil
}

func executarIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	referencia := fs.String("lote", "", "referencia do lote (AAAA-MM); vazio = mais recente")
	diretorio := fs.String("dir", "", "onde guardar os zips entre execucoes (padrao: "+ingest.DiretorioPadrao+"); precisa persistir para a retomada funcionar")
	dryRun := fs.Bool("dry-run", false, "listar o que seria feito, sem baixar nem gravar")
	cnaes := fs.String("cnae", "", "carregar so estabelecimentos destes CNAEs, separados por virgula (vazio = todos)")
	cnaeSec := fs.Bool("cnae-secundario", false, "com -cnae, inclui quem tem o codigo como atividade secundaria")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dsn := os.Getenv("FONTECA_DSN")
	if dsn == "" {
		return fmt.Errorf("FONTECA_DSN nao definida")
	}

	ctx := context.Background()
	pool, err := store.Conectar(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	opts := ingest.Opcoes{
		Referencia: *referencia,
		Diretorio:  *diretorio,
		DryRun:     *dryRun,
	}
	if *cnaes != "" {
		opts.CNAEs = strings.Split(*cnaes, ",")
		opts.CNAESecundario = *cnaeSec
	}
	return ingest.Executar(ctx, pool, opts)
}

func executarExport(args []string) (err error) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	cnaes := fs.String("cnae", "", "CNAEs separados por virgula (obrigatorio)")
	ufs := fs.String("uf", "", "UFs separadas por virgula")
	dias := fs.Int("ultimos-dias", 0, "abertas nos ultimos N dias (0 = sem limite)")
	soCelular := fs.Bool("so-celular", false, "apenas com telefone celular")
	semMEI := fs.Bool("sem-mei", false, "excluir optantes pelo MEI")
	limite := fs.Int("limite", store.LimiteMaximo, "maximo de linhas")
	saida := fs.String("o", "", "arquivo de saida (vazio = stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cnaes == "" {
		return fmt.Errorf("-cnae e obrigatorio (ex: -cnae 2512800,2511000)")
	}

	dsn := os.Getenv("FONTECA_DSN")
	if dsn == "" {
		return fmt.Errorf("FONTECA_DSN nao definida")
	}

	ctx := context.Background()
	pool, err := store.Conectar(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	filtro := store.Filtro{
		CNAEs:       strings.Split(*cnaes, ","),
		Situacao:    store.SituacaoAtiva,
		UltimosDias: *dias,
		SoCelular:   *soCelular,
		ExcluirMEI:  *semMEI,
		Limite:      *limite,
	}
	if *ufs != "" {
		filtro.UFs = strings.Split(*ufs, ",")
	}

	rows, err := store.Buscar(ctx, pool, filtro)
	if err != nil {
		return err
	}
	defer rows.Close()

	destino := io.Writer(os.Stdout)
	if *saida != "" {
		arq, erroCriar := os.Create(*saida)
		if erroCriar != nil {
			return fmt.Errorf("criar %s: %w", *saida, erroCriar)
		}
		defer func() {
			// Erro no Close pode significar CSV truncado em disco cheio.
			if cerr := arq.Close(); cerr != nil && err == nil {
				err = fmt.Errorf("fechar %s: %w", *saida, cerr)
			}
		}()
		destino = arq
	}

	n, err := export.EscreverCSV(destino, rows)
	if err != nil {
		return err
	}
	// A contagem vai para stderr para nao sujar o CSV quando sai em stdout.
	fmt.Fprintf(os.Stderr, "%d linha(s) exportada(s)\n", n)
	return nil
}

func executarServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	endereco := fs.String("addr", ":8080", "endereco de escuta")
	porMinuto := fs.Int("rate-minuto", 60, "requisicoes por minuto, por conta")
	porDia := fs.Int("rate-dia", 10000, "requisicoes por dia, por conta")
	porIP := fs.Int("rate-ip", 10, "falhas de autenticacao por minuto, por IP")
	proxy := fs.String("proxy-confiavel", "", "IP do proxy reverso; so dele o X-Forwarded-For e lido (vazio = nunca)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proxy != "" && net.ParseIP(*proxy) == nil {
		return fmt.Errorf("-proxy-confiavel %q nao e um IP", *proxy)
	}

	// Sem fallback para FONTECA_DSN: aquele e o dono das tabelas, e sob o dono
	// a RLS de meta.uso fica inerte. A API so sobe como fonteca_api.
	dsn := os.Getenv("FONTECA_DSN_API")
	if dsn == "" {
		return fmt.Errorf("FONTECA_DSN_API nao definida: a API precisa do usuario fonteca_api (deploy/papeis.sql), nunca do dono das tabelas")
	}

	ctx := context.Background()
	pool, err := store.Conectar(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := store.VerificarPapelSeguro(ctx, pool); err != nil {
		return fmt.Errorf("%w; use o usuario fonteca_api de deploy/papeis.sql em FONTECA_DSN_API", err)
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	// responderJSON e outros caminhos usam slog.Default: sem isto sairiam em
	// texto, fora do log estruturado.
	slog.SetDefault(log)
	handler := api.Novo(pool, api.Opcoes{
		Log:       log,
		PorMinuto: *porMinuto,
		PorDia:    *porDia,
		PorIP:     *porIP,

		ProxyConfiavel: *proxy,
	})

	srv := &http.Server{
		Addr:              *endereco,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Encerramento gracioso: requisicao em curso termina antes de fechar.
	parar := make(chan os.Signal, 1)
	signal.Notify(parar, os.Interrupt, syscall.SIGTERM)
	erroServidor := make(chan error, 1)

	go func() {
		log.Info("api ouvindo", "endereco", *endereco)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erroServidor <- err
		}
	}()

	select {
	case <-parar:
		log.Info("encerrando")
		ctxParada, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancelar()
		return srv.Shutdown(ctxParada)
	case err := <-erroServidor:
		return fmt.Errorf("servidor: %w", err)
	}
}

func executarConta(args []string) error {
	fs := flag.NewFlagSet("conta", flag.ExitOnError)
	nome := fs.String("nome", "", "nome da conta (obrigatorio)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nome == "" {
		return fmt.Errorf("-nome e obrigatorio")
	}

	dsn := os.Getenv("FONTECA_DSN")
	if dsn == "" {
		return fmt.Errorf("FONTECA_DSN nao definida")
	}

	ctx := context.Background()
	pool, err := store.Conectar(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	contaID, chave, err := tenant.CriarConta(ctx, pool, *nome)
	if err != nil {
		return err
	}

	fmt.Printf("conta:  %s\nchave:  %s\n\n", contaID, chave)
	fmt.Fprintln(os.Stderr, "Guarde a chave agora: so o hash fica no banco e ela nao pode ser recuperada.")
	return nil
}

// executarIBGE grava meta.municipio_ibge. Nao depende de lote: o de-para e
// permanente, entao basta rodar uma vez (e de novo se o Tesouro atualizar).
func executarIBGE(args []string) error {
	fs := flag.NewFlagSet("ibge", flag.ExitOnError)
	arquivo := fs.String("arquivo", "", "TABMUN.csv local (padrao: baixa do Tesouro Transparente)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dsn := os.Getenv("FONTECA_DSN")
	if dsn == "" {
		return fmt.Errorf("FONTECA_DSN nao definida")
	}

	ctx := context.Background()
	var fonte io.ReadCloser
	if *arquivo != "" {
		arq, err := os.Open(*arquivo)
		if err != nil {
			return err
		}
		fonte = arq
	} else {
		corpo, err := ingest.BaixarTabMun(ctx, ingest.URLTabMun)
		if err != nil {
			return err
		}
		fonte = corpo
	}
	defer fonte.Close()

	pool, err := store.Conectar(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.MigrarMeta(ctx, pool); err != nil {
		return err
	}

	n, err := ingest.CarregarIBGE(ctx, pool, fonte)
	if err != nil {
		return err
	}
	fmt.Printf("de-para SIAFI -> IBGE: %d municipios gravados\n", n)
	return nil
}
