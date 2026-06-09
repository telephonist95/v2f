#import "@docs/gost732-2017:0.5.0": *
#import "@docs/bmstu:0.4.0": *
#show: гост732-2017
#show " –": "\u{00a0}–"
#show " —": "\u{00a0}—"

#set page(footer: context {
  set text(size: 14pt)
  set align(center)
  let cur = counter(page).get().first()
  let covers = query(selector(<appendix-cover>).before(here()))
  let is_cover = covers.len() > 0 and covers.last().location().page() == here().page()
  let n = if is_cover {
    cur
  } else {
    let starts = query(selector(<appendix-content-start>).before(here()))
    if starts.len() > 0 {
      let start_page = counter(page).at(starts.last().location()).first()
      cur - start_page + 1
    } else {
      cur
    }
  }
  [#n]
})

#show outline.entry: it => {
  let el = it.element
  let body-text = lower(repr(el.body)).trim()
  if body-text.contains("приложение") {
    let text = if el.supplement == [Раздел] { el.body } else { el.supplement }
    link(el.location())[
      #it.indented(none, [ #text #box(width: 1fr, it.fill) #it.page() ])
    ]
  } else {
    it
  }
}
#metadata(true) <gost732-2017-feature-table-head-small-spacing>


#show figure.where(kind: table): it => {
  set block(breakable: true)
  set figure.caption(position: top)
  show figure.caption: set align(left)
  let continuation = counter("continuation")
  table(
    stroke: 0em,
    inset: (x: 0em, y: 0em),
    columns: (1fr),
    table.header(table.cell(inset: (bottom: 0.2em))[#align(left)[
      #context if continuation.get().at(0) == 0 {
        continuation.update(1)
        it.caption
      } else {
        set par(justify: false, leading: 0.65em, first-line-indent: 0cm)
        set text(size: 14pt)
        [Продолжение таблицы #counter(figure.where(kind: table)).display()]
      }
    ]]),
    [#it.body]
  )
  context continuation.update(0)
  v(1em)
}

#show figure.where(kind: raw): it => {
  set block(breakable: true)
  set figure.caption(position: top)
  show figure.caption: set align(left)
  let continuation = counter("continuation")
  table(
    stroke: 0em,
    inset: (x: 0em, y: 0em),
    columns: (1fr),
    table.header(table.cell(inset: (bottom: 0.2em))[#align(left)[
      #context if continuation.get().at(0) == 0 {
        continuation.update(1)
        it.caption
      } else {
        set par(justify: false, leading: 0.65em, first-line-indent: 0cm)
        set text(size: 14pt)
        [Продолжение листинга #counter(figure.where(kind: raw)).display()]
      }
    ]]),
    [#it.body]
  )
  context continuation.update(0)
  v(1em)
}

#show list: it => {
  h(-1.25cm)
  for phase in it.children [
    #h(1.25cm)--#h(0.5em)#phase.body \
  ]
}

#show enum: it => {
  let i = 1
  h(-1.25cm)
  for phase in it.children [
    #h(1.25cm)#i)#h(0.5em)#phase.body \
    #{i = i + 1}
  ]
}

// Сборка для сайта кафедры: typst compile --input signed=true main.typ
// При signed=true титул/задание/план/обложки приложений берутся из цветных
// сканов подписанных документов (img/signed/), иначе — векторная вёрстка.
#let _signed = sys.inputs.at("signed", default: "") == "true"

#if _signed [
  // Титул на весь лист, без полей (скан подогнан под пропорции A4).
  #page(
    image("/img/signed_full/title.jpg", width: 100%),
    margin: 0pt,
    footer: none,
  )
] else [
#page(margin: (left: 30mm, right: 15mm, top: 20mm, bottom: 15mm), footer: [])[
  #set text(font: "Times New Roman", size: 14pt, lang: "ru")
  #set par(first-line-indent: 0em)

  #mk_title_header()
  #mk_title_header_row("Факультет", "Информатика и системы управления")
  #mk_title_header_row("Кафедра", "Компьютерные системы и сети")
  #mk_title_header_row("Направление подготовки", "09.03.01 Информатика и вычислительная техника", no_upper: true)

  #v(1fr)

  #align(center)[
    #text(size: 20pt, weight: "bold")[РАСЧЕТНО-ПОЯСНИТЕЛЬНАЯ ЗАПИСКА]
    #v(0.3em)
    #par(leading: 0.65em)[
      #text(size: 16pt, weight: "bold", style: "italic")[
        К ВЫПУСКНОЙ КВАЛИФИКАЦИОННОЙ \
        РАБОТЕ БАКАЛАВРА НА ТЕМУ:
      ]
    ]
    #v(0.5em)
    #set align(left)
    #par(justify: true)[
      #set text(size: 18pt, weight: "bold", style: "italic")
      #underline(evade: false, offset: 3pt)[
        Система высокоуровневого имитационного моделирования цифровых устройств
      ]
    ]
  ]

  #v(1fr)

  #[
    #set text(14pt, weight: "regular", hyphenate: false)
    #let underlined(content, label) = align(center)[
      #content
      #v(-12pt)
      #line(length: 100%, stroke: 1pt)
      #v(-12pt)
      #text(size: 10pt)[(#label)]
    ]
    #let sign-row(role, name, group: none) = grid(
      columns: (150pt, 1fr, 3fr),
      gutter: 30pt,
      align(left)[#role #v(12pt)],
      if group != none {
        underlined(group, "Группа")
      } else { [] },
      grid(
        columns: (1fr, 1fr),
        gutter: 20pt,
        underlined(hide[Пд], "Подпись, дата"),
        underlined(name, "И.О. Фамилия"),
      ),
    )
    #sign-row("Студент", "О.В. Жданов", group: "ИУ6-82Б")
    #sign-row("Руководитель ВКРБ", "А.Ю. Попов")
    #sign-row("Нормоконтролер", "О.Ю. Ерёмин")
  ]

  #v(1fr)
  #align(center)[#text(14pt, style: "italic")[2026 г.]]
  #v(5mm)
]
]

// Логотип МГТУ в шапке титула (helper bmstu) вставляется как figure(kind: image)
// без подписи и съедает «Рисунок 1». Обнуляем счётчик рисунков перед телом, чтобы
// первый настоящий рисунок получил номер 1.
#counter(figure.where(kind: image)).update(0)

#page(align(left, image(if _signed { "/img/signed_full/zadanie.jpg" } else { "img/task1_cropped.jpg" }, width: 100%)), margin: if _signed { 0pt } else { (left: 3cm, right: 1.5cm, top: 2cm, bottom: 2cm) }, footer: if _signed { none } else { align(center)[#text(fill: white)[1]] })
#page(align(left, image(if _signed { "/img/signed_full/zadanie2.jpg" } else { "img/task2_cropped.jpg" }, width: 100%)), margin: if _signed { 0pt } else { (left: 3cm, right: 1.5cm, top: 2cm, bottom: 2cm) }, footer: if _signed { none } else { align(center)[#text(fill: white)[1]] })
#page(align(left, image(if _signed { "/img/signed_full/plan.jpg" } else { "img/plan_croped.jpg" }, width: 100%)), margin: if _signed { 0pt } else { (left: 3cm, right: 1.5cm, top: 2cm, bottom: 2cm) }, footer: if _signed { none } else { align(center)[#text(fill: white)[1]] })
#counter(page).update(n => n - 1)

#include "00a_annotation.typ"
#include "00_abstract.typ"
#содержание()
#include "01_abbreviations.typ"
#include "02_introduction.typ"

#include "03_theory.typ"
#include "04_tools_analysis.typ"

#include "05_architecture.typ"
#include "06_converter.typ"
#include "07_rtl_abstractor.typ"

#include "08_testing.typ"

#include "09_conclusion.typ"
#set bibliography(style: "bib.csl", full: нет)
#show bibliography: it_bib => {
  set block(inset: 0pt)
  show block: it_block => {
    par(it_block.body)
  }
  it_bib
}
#bibliography("bib.yml")

#include "10_appendix_a_tz.typ"
#include "11_appendix_b_user_guide.typ"
#include "12_appendix_c_listing.typ"
#include "13_appendix_g_graphics.typ"
