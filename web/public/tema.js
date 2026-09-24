// Aplica o tema antes da primeira pintura; sem isso a tela pisca no tema
// errado ao carregar. Escuro e o padrao da marca.
try {
  document.documentElement.dataset.tema = localStorage.getItem('fonteca.tema') || 'escuro'
} catch {
  document.documentElement.dataset.tema = 'escuro'
}
