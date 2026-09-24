"""Gera web/src/dados/mapa-uf.json a partir da malha de UFs do IBGE.

Uso: python web/scripts/gerar-mapa-uf.py [arquivo.svg]
Sem argumento, baixa de servicodados.ibge.gov.br (malha oficial, qualidade
minima — suficiente para um mapa de 500px). Saida: viewBox 0..1000, um path
por UF com coordenadas inteiras e o centro visual de cada estado, onde o
radar acende o ponto.
"""

import json
import re
import sys
import urllib.request
from pathlib import Path

URL = (
    "https://servicodados.ibge.gov.br/api/v3/malhas/paises/BR"
    "?formato=image/svg+xml&qualidade=minima&intrarregiao=UF"
)

SIGLAS = {
    "11": "RO", "12": "AC", "13": "AM", "14": "RR", "15": "PA", "16": "AP", "17": "TO",
    "21": "MA", "22": "PI", "23": "CE", "24": "RN", "25": "PB", "26": "PE", "27": "AL",
    "28": "SE", "29": "BA", "31": "MG", "32": "ES", "33": "RJ", "35": "SP", "41": "PR",
    "42": "SC", "43": "RS", "50": "MS", "51": "MT", "52": "GO", "53": "DF",
}

LADO = 1000
MARGEM = 10


def poligonos(d: str):
    """Le "M x,y l dx,dy h dx ... z" em listas de pontos absolutos."""
    fichas = re.findall(r"[MmLlHhVvZz]|-?\d+(?:\.\d+)?", d)
    polis, atual, x, y, cmd, i = [], [], 0.0, 0.0, "M", 0
    while i < len(fichas):
        f = fichas[i]
        if f in "MmLlHhVvZz":
            cmd = f
            i += 1
            if f in "Zz":
                if atual:
                    polis.append(atual)
                atual = []
            continue
        if cmd in "HhVv":
            v = float(f)
            i += 1
            if cmd == "H":
                x = v
            elif cmd == "h":
                x += v
            elif cmd == "V":
                y = v
            else:
                y += v
            atual.append((x, y))
            continue
        dx, dy = float(fichas[i]), float(fichas[i + 1])
        i += 2
        if cmd in "M":
            x, y = dx, dy
            if atual:
                polis.append(atual)
            atual = [(x, y)]
            cmd = "L"
        elif cmd == "m":
            x, y = x + dx, y + dy
            if atual:
                polis.append(atual)
            atual = [(x, y)]
            cmd = "l"
        elif cmd == "L":
            x, y = dx, dy
            atual.append((x, y))
        else:
            x, y = x + dx, y + dy
            atual.append((x, y))
    if atual:
        polis.append(atual)
    return polis


def area_e_centro(p):
    a = cx = cy = 0.0
    for (x1, y1), (x2, y2) in zip(p, p[1:] + p[:1]):
        c = x1 * y2 - x2 * y1
        a += c
        cx += (x1 + x2) * c
        cy += (y1 + y2) * c
    if a == 0:
        return 0, p[0]
    return a / 2, (cx / (3 * a), cy / (3 * a))


def main() -> None:
    if len(sys.argv) > 1:
        svg = Path(sys.argv[1]).read_text(encoding="utf-8")
    else:
        with urllib.request.urlopen(URL, timeout=120) as r:
            svg = r.read().decode("utf-8")

    estados = {}
    for codigo, d in re.findall(r'<path id="(\d+)" d="([^"]+)"', svg):
        # transform do IBGE: scale(0.0001, -0.0001) — y cresce para cima.
        estados[SIGLAS[codigo]] = [[(x * 1e-4, -y * 1e-4) for x, y in p] for p in poligonos(d)]

    xs = [x for ps in estados.values() for p in ps for x, _ in p]
    ys = [y for ps in estados.values() for p in ps for _, y in p]
    minx, miny = min(xs), min(ys)
    k = (LADO - 2 * MARGEM) / max(max(xs) - minx, max(ys) - miny)
    largura = round((max(xs) - minx) * k + 2 * MARGEM)
    altura = round((max(ys) - miny) * k + 2 * MARGEM)

    def t(pt):
        return (round((pt[0] - minx) * k + MARGEM, 1), round((pt[1] - miny) * k + MARGEM, 1))

    saida = {"viewBox": f"0 0 {largura} {altura}", "estados": {}}
    for uf, ps in sorted(estados.items()):
        partes, maior = [], (0, (0, 0))
        for p in ps:
            q = [t(pt) for pt in p]
            # Descarta vertices a menos de 1,2 unidade do anterior: some ruido
            # sem mudar o desenho a 500px.
            enxuto = [q[0]]
            for pt in q[1:]:
                if abs(pt[0] - enxuto[-1][0]) + abs(pt[1] - enxuto[-1][1]) >= 1.2:
                    enxuto.append(pt)
            if len(enxuto) < 3:
                continue
            a, c = area_e_centro(enxuto)
            if abs(a) > abs(maior[0]):
                maior = (a, c)
            partes.append("M" + "L".join(f"{x:g},{y:g}" for x, y in enxuto) + "Z")
        saida["estados"][uf] = {
            "d": "".join(partes),
            "centro": [round(maior[1][0], 1), round(maior[1][1], 1)],
        }

    destino = Path(__file__).resolve().parent.parent / "src" / "dados" / "mapa-uf.json"
    destino.write_text(json.dumps(saida, separators=(",", ":")), encoding="utf-8")
    print(f"{len(saida['estados'])} UFs, viewBox {saida['viewBox']} -> {destino} ({destino.stat().st_size // 1024} KB)")


if __name__ == "__main__":
    main()
