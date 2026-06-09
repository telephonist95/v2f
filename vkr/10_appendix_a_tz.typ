#import "@docs/gost732-2017:0.5.0": *

#let _signed = sys.inputs.at("signed", default: "") == "true"

#ненумерованный_заголовок(содержание: [ПРИЛОЖЕНИЕ А. Техническое задание])[Приложение А]

#metadata("cover") <appendix-cover>

#align(center)[
  #text(size: 14pt, weight: "bold")[Техническое задание]

  Листов 9
]

#pagebreak()
#metadata("content-start") <appendix-content-start>

#set heading(outlined: false)
#set figure(numbering: (.., n) => [А.#n])

// Остальные страницы ТЗ — напрямую из PDF (docs/Техническое задание.pdf →
// img/tz.pdf): каждая страница кладётся на отдельный лист без полей, поэтому
// сохраняются исходные поля и собственная нумерация ТЗ, а векторный текст
// остаётся чётким (в отличие от растровых сканов).
#let страница_тз(n) = page(
  margin: 0pt,
  footer: none,
  image("img/tz.pdf", page: n, width: 100%),
)

// Титул ТЗ — скан с реальными подписями (та же схема, что страницы задания).
#page(align(left, image(if _signed { "/img/signed_full/prilozhenie_a.jpg" } else { "img/tz/page-1_cropped.png" }, width: 100%)), margin: if _signed { 0pt } else { (left: 3cm, right: 1.5cm, top: 2cm, bottom: 2cm) }, footer: if _signed { none } else { align(center)[#text(fill: white)[1]] })
#страница_тз(2)
#страница_тз(3)
#страница_тз(4)
#страница_тз(5)
#страница_тз(6)
#страница_тз(7)
#страница_тз(8)
#страница_тз(9)
