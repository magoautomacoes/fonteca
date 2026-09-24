"""Gera web/src/dados/cnae.json a partir da API de CNAE do IBGE (CONCLA).

Uso: python web/scripts/gerar-catalogo-cnae.py [arquivo.json]
Sem argumento, baixa de servicodados.ibge.gov.br. A CNAE muda raramente
(versao 2.3 desde 2019); rode de novo quando o IBGE publicar revisao.

Saida compacta, para o navegador baixar uma vez:
  secoes:     { "C": "Indústrias de transformação", ... }
  divisoes:   { "25": ["Fabricação de produtos de metal...", "C"], ... }
  grupos:     { "251": "Fabricação de estruturas metálicas...", ... }
  classes:    { "10911": "Fabricação de produtos de panificação", ... }
  subclasses: [["1091102", "Fabricação de produtos de padaria...", "termos..."], ...]
Os termos sao as atividades populares do IBGE ("padaria", "serralheria"),
sem acento e em minusculas, so para a busca.
"""

import json
import sys
import unicodedata
import urllib.request
from pathlib import Path

URL = "https://servicodados.ibge.gov.br/api/v2/cnae/subclasses"

# Siglas e nomes que nao podem virar minuscula ao sair da caixa alta.
PRESERVAR = {"sus", "tv", "cd", "dvd", "gnv", "glp", "ong", "ongs", "pvc", "ii", "iii"}

# Palavras que nao ajudam a achar a atividade.
VAZIAS = set(
    "de da do das dos e em com sem para por a o as os ao aos na no nas nos ou "
    "fabricacao comercio varejista atacadista servicos servico exceto outros outras "
    "quando realizada pela unidade similares semelhantes produtos produto qualquer "
    "material tipo etc".split()
)


def frase(texto: str) -> str:
    """ "COMÉRCIO VAREJISTA DE FERRAGENS" -> "Comércio varejista de ferragens"."""
    palavras = texto.strip().lower().split()
    saida = []
    for p in palavras:
        nua = p.strip(",.;:()")
        saida.append(p.upper() if nua in PRESERVAR else p)
    s = " ".join(saida)
    return s[:1].upper() + s[1:]


def sem_acento(texto: str) -> str:
    n = unicodedata.normalize("NFD", texto.lower())
    return "".join(c for c in n if unicodedata.category(c) != "Mn")


def main() -> None:
    if len(sys.argv) > 1:
        dados = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
    else:
        with urllib.request.urlopen(URL, timeout=120) as r:
            dados = json.loads(r.read().decode("utf-8"))

    secoes, divisoes, grupos, classes, subclasses = {}, {}, {}, {}, []
    for s in dados:
        classe = s["classe"]
        grupo = classe["grupo"]
        divisao = grupo["divisao"]
        secao = divisao["secao"]
        secoes[secao["id"]] = frase(secao["descricao"])
        divisoes[divisao["id"]] = [frase(divisao["descricao"]), secao["id"]]
        grupos[grupo["id"]] = frase(grupo["descricao"])
        classes[classe["id"]] = frase(classe["descricao"])
        # So as palavras que a descricao ainda nao tem: "padaria" acha a
        # subclasse cujo nome oficial e "produtos de padaria e confeitaria".
        ja = set(sem_acento(s["descricao"]).replace(",", " ").split())
        palavras = set()
        for t in s.get("atividades") or []:
            for p in sem_acento(t).replace(",", " ").replace(";", " ").replace("(", " ").replace(")", " ").split():
                p = p.strip(".-/'\"")
                if len(p) >= 3 and p not in VAZIAS and p not in ja and not p.isdigit():
                    palavras.add(p)
        subclasses.append([s["id"], frase(s["descricao"]), " ".join(sorted(palavras))])

    subclasses.sort(key=lambda x: x[0])
    saida = {
        "fonte": "IBGE/CONCLA, CNAE 2.3 subclasses",
        "secoes": dict(sorted(secoes.items())),
        "divisoes": dict(sorted(divisoes.items())),
        "grupos": dict(sorted(grupos.items())),
        "classes": dict(sorted(classes.items())),
        "subclasses": subclasses,
    }
    destino = Path(__file__).resolve().parent.parent / "src" / "dados" / "cnae.json"
    destino.parent.mkdir(parents=True, exist_ok=True)
    destino.write_text(json.dumps(saida, ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
    print(f"{len(subclasses)} subclasses, {len(divisoes)} divisoes -> {destino} ({destino.stat().st_size // 1024} KB)")


if __name__ == "__main__":
    main()
