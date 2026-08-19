#!/usr/bin/env node
// Build the Quran topical index seed from the masjidtucson.org HTML pages.
//
// Source: https://www.masjidtucson.org/quran/index/qindex_{abc,def,ghi,jklm,nopqr,stu,vwxyz}.html
// These are the printed Quran's alphabetical subject index, marked up in 1990s
// Dreamweaver HTML: entries are bare <p> tags, sub-entries are unclosed <li>
// items, and the <ul> open/close counts do not balance. The parser below is
// deliberately tolerant — it never assumes well-formed markup.
//
// Output: db/seeds/quran_topic_index_en.json (consumed by db/topic_index.go).
//
// Usage:
//   node scripts/topic_index/build.mjs            # parse the cached raw/ pages
//   node scripts/topic_index/build.mjs --fetch    # re-download raw/ first
//
// Verse numbers are emitted 1-based, exactly as printed. Note that the
// `verses.verse_index` column in Postgres is 0-based; do not conflate them.

import { readFile, writeFile, mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const RAW_DIR = path.join(HERE, 'raw')
const OUT_FILE = path.join(HERE, '..', '..', 'db', 'seeds', 'quran_topic_index_en.json')

const PAGES = ['abc', 'def', 'ghi', 'jklm', 'nopqr', 'stu', 'vwxyz']
const SOURCE_BASE = 'https://www.masjidtucson.org/quran/index'

// Verse counts per chapter, index 0 = chapter 1. Used only to validate refs.
const VERSE_COUNTS = [
  7, 286, 200, 176, 120, 165, 206, 75, 127, 109, 123, 111, 43, 52, 99, 128,
  111, 110, 98, 135, 112, 78, 118, 64, 77, 227, 93, 88, 69, 60, 34, 30, 73,
  54, 45, 83, 182, 88, 75, 85, 54, 53, 89, 59, 37, 35, 38, 29, 18, 45, 60,
  49, 62, 55, 78, 96, 29, 22, 24, 13, 14, 11, 11, 18, 12, 12, 30, 52, 52,
  44, 28, 28, 20, 56, 40, 31, 50, 40, 46, 42, 29, 19, 36, 25, 22, 17, 19,
  26, 30, 20, 15, 21, 11, 8, 8, 19, 5, 8, 8, 11, 11, 8, 3, 9, 5, 4, 7, 3,
  6, 3, 5, 4, 5, 6,
]

const anomalies = []
function anomaly(kind, context, detail) {
  anomalies.push({ kind, context, detail })
}

// ── HTML helpers ────────────────────────────────────────────────────────────

const ENTITIES = {
  amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ',
  eacute: 'é', egrave: 'è', agrave: 'à', ccedil: 'ç', uuml: 'ü',
  ouml: 'ö', auml: 'ä', ntilde: 'ñ', iacute: 'í', oacute: 'ó',
  aacute: 'á', uacute: 'ú', mdash: '-', ndash: '-', hellip: '...',
  rsquo: "'", lsquo: "'", ldquo: '"', rdquo: '"',
}

function decodeEntities(s) {
  return s
    .replace(/&#(\d+);/g, (_, d) => String.fromCharCode(Number(d)))
    .replace(/&#x([0-9a-f]+);/gi, (_, h) => String.fromCharCode(parseInt(h, 16)))
    .replace(/&([a-z]+);/gi, (m, name) => {
      const v = ENTITIES[name.toLowerCase()]
      return v === undefined ? m : v
    })
}

function stripTags(s) {
  return collapse(decodeEntities(s.replace(/<[^>]*>/g, ' ')))
}

function collapse(s) {
  return s.replace(/\s+/g, ' ').trim()
}

// ── Verse reference parsing ─────────────────────────────────────────────────
//
// A reference run looks like:
//   <a href="ch7.html#65">7:65</a>-72; <a href="ch11.html#50">11:50</a>-60
//   <a href="ch3.html#67">3:67</a>,95
//   <a href="ch2.html#17">2:17</a>,19-20,24-26,265
//
// The anchor carries the chapter and the first verse. The plain text that
// follows, up to the next anchor, extends that chapter: `-N` closes a range,
// `,N` adds a verse, `,N-M` adds a nested range. A `;` ends the run for that
// chapter, so anything after it is ignored (the next chapter has its own anchor).

const ANCHOR_RE = /<a\s+href="ch(\d+)\.html#(\d+)"[^>]*>([^<]*)<\/a>/gi

function parseRefs(html, context) {
  const refs = []
  const matches = [...html.matchAll(ANCHOR_RE)]

  for (let i = 0; i < matches.length; i++) {
    const m = matches[i]
    const chapter = Number(m[1])
    const startVerse = Number(m[2])
    const label = collapse(decodeEntities(m[3]))

    // The plain text between this anchor and the next one (or end of block).
    const tailStart = m.index + m[0].length
    const tailEnd = i + 1 < matches.length ? matches[i + 1].index : html.length
    const rawTail = html.slice(tailStart, tailEnd)

    // A `;` (or a `<br>`, `<li>`, `</ul>`) terminates this chapter's run.
    const tail = collapse(decodeEntities(rawTail.split(/[;<]/, 1)[0] ?? ''))

    // The anchor label should read "chapter:verse". When it does not, the
    // source markup is broken — record it and fall back to the href.
    // A bare-number label ("60") is a legitimate same-chapter continuation of the
    // previous reference; only a label that disagrees with its href is a problem.
    const labelMatch = /^(?:(\d+)\s*:\s*)?(\d+)$/.exec(label)
    if (!labelMatch) {
      anomaly('anchor-label', context, `href says ch${chapter}#${startVerse}, label reads "${label}"`)
    } else if (
      (labelMatch[1] !== undefined && Number(labelMatch[1]) !== chapter) ||
      Number(labelMatch[2]) !== startVerse
    ) {
      anomaly('anchor-mismatch', context, `href ch${chapter}#${startVerse} vs label "${label}"`)
    }

    const segments = parseTail(chapter, startVerse, tail, context)
    for (const seg of segments) refs.push(seg)
  }

  return refs
}

// The printed index abbreviates a range end to its trailing digits: "9:36-7"
// means 9:36-37, "26:198-200" is already complete. Expand an end that reads as
// smaller than its start by borrowing the start's leading digits.
function expandRangeEnd(start, end) {
  if (end >= start) return end
  const s = String(start)
  const e = String(end)
  for (let keep = s.length - e.length; keep > 0; keep--) {
    const candidate = Number(s.slice(0, keep) + e)
    if (candidate > start) return candidate
  }
  return end
}

// Turns one chapter's run into { chapter, verse_start, verse_end, display }
// segments. The first segment always starts at the anchor's verse.
function parseTail(chapter, startVerse, tail, context) {
  const out = []
  const cursor = { start: startVerse, end: startVerse }

  // Leading `-N` closes the range opened by the anchor.
  let rest = tail
  const leadRange = /^-\s*(\d+)/.exec(rest)
  if (leadRange) {
    cursor.end = expandRangeEnd(startVerse, Number(leadRange[1]))
    rest = rest.slice(leadRange[0].length)
  }
  out.push(cursor)

  // Remaining `,N` / `,N-M` items add further verses in the same chapter.
  // The source uses `.` for `,` in a handful of places ("16:11.67") and now and
  // then omits the separator entirely — accept both, noting the latter.
  let run = /^\s*[,.]\s*([\d\s,.\-]+)/.exec(rest)
  if (!run) {
    const bare = /^(\d[\d\s,.\-]*)/.exec(rest)
    if (bare) {
      anomaly('missing-separator', context, `"${collapse(bare[1])}" follows a reference with no comma`)
      run = bare
    }
  }

  if (run) {
    for (const piece of run[1].split(/[,.]/)) {
      const p = piece.trim()
      if (!p) continue
      const range = /^(\d+)\s*-\s*(\d+)$/.exec(p)
      const single = /^(\d+)$/.exec(p)
      if (range) {
        const start = Number(range[1])
        out.push({ start, end: expandRangeEnd(start, Number(range[2])) })
      } else if (single) {
        out.push({ start: Number(single[1]), end: Number(single[1]) })
      } else {
        anomaly('tail-token', context, `unparsed "${p}" in tail "${tail}"`)
      }
    }
    rest = rest.slice(run[0].length)
  }

  const leftover = rest.trim()
  if (leftover && !/^[\s,.;:]*$/.test(leftover)) {
    if (/^\(?see\b/i.test(leftover)) {
      // A cross-reference parenthetical trailing the references — lifted into
      // cross_refs by parseEntry, so nothing is lost here.
    } else if (/appendix/i.test(leftover)) {
      // The printed index sometimes points at an appendix rather than a verse.
      // Appendices are a separate corpus (/appendices), not modelled here.
      anomaly('appendix-reference', context, `"${leftover}" points at an appendix, not a verse`)
    } else {
      anomaly('tail-remainder', context, `leftover "${leftover}" in tail "${tail}"`)
    }
  }

  const max = VERSE_COUNTS[chapter - 1]
  // Verse 0 is the un-numbered Basmalah that opens every chapter except 1 and 9.
  const floor = chapter === 1 || chapter === 9 ? 1 : 0
  return out
    .filter((seg) => {
      if (chapter < 1 || chapter > 114) {
        anomaly('bad-chapter', context, `chapter ${chapter}`)
        return false
      }
      if (seg.start < floor || seg.end < seg.start || seg.end > max) {
        anomaly('bad-verse', context, `${chapter}:${seg.start}-${seg.end} (chapter has ${max} verses)`)
        return false
      }
      return true
    })
    .map((seg) => ({
      chapter_number: chapter,
      verse_start: seg.start,
      verse_end: seg.end,
      display: seg.start === seg.end ? `${chapter}:${seg.start}` : `${chapter}:${seg.start}-${seg.end}`,
    }))
}

// ── Cross-reference extraction ──────────────────────────────────────────────
//
// Titles carry their cross-references inline, in four shapes seen in the source:
//   Aad (see also Hood)          Demons (see Jinns)
//   Agreements see Covenant      Baby,see Infants
// Everything after the marker is a comma-separated list of other entry titles.

const CROSS_PATTERNS = [
  /\s*\((?:see\s+also|see)\s+([^)]*)\)\s*$/i,
  /\s*,?\s*\bsee\s+also\s+(.+)$/i,
  /\s*,?\s*\bsee\s+(.+)$/i,
]

function splitCrossRefs(title) {
  for (const re of CROSS_PATTERNS) {
    const m = re.exec(title)
    if (!m) continue
    const refs = m[1]
      .split(/,|\band\b|&/)
      .map((s) => collapse(s))
      .filter((s) => s.length > 0 && s.toLowerCase() !== 'specific names')
    return { title: collapse(title.slice(0, m.index)), cross_refs: refs }
  }
  return { title: collapse(title), cross_refs: [] }
}

// ── Sort key ────────────────────────────────────────────────────────────────
//
// "Almighty,God is" sorts under A by its leading word; "Al-Baqara" likewise.
// Lowercase, drop everything but letters, digits and spaces.

function sortKey(title) {
  return title
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9 ]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function slugify(title) {
  return sortKey(title).replace(/ /g, '-').slice(0, 80)
}

// ── Page parsing ────────────────────────────────────────────────────────────

// Each letter section starts at <a name="AA"></a> and runs to the next one.
const LETTER_RE = /<a\s+name="([A-Z])\1"\s*>\s*<\/a\s*>/gi

function parsePage(html, pageName) {
  const entries = []
  const marks = [...html.matchAll(LETTER_RE)]
  if (marks.length === 0) {
    anomaly('no-letters', pageName, 'no letter anchors found on page')
    return entries
  }

  for (let i = 0; i < marks.length; i++) {
    const letter = marks[i][1].toUpperCase()
    const start = marks[i].index + marks[i][0].length
    const end = i + 1 < marks.length ? marks[i + 1].index : html.length
    let section = html.slice(start, end)

    // Drop the trailing page furniture (footer table, mailto, hr rules).
    const footer = section.search(/<table|mailto:/i)
    if (footer !== -1) section = section.slice(0, footer)

    for (const entry of parseSection(section, letter, pageName)) {
      entries.push(entry)
    }
  }

  return entries
}

function parseSection(section, letter, pageName) {
  const entries = []

  // Top-level entries open with <p> — except where the source forgot it and left
  // the title bare between a closing </ul> and the next <ul> ("Aaron (see also
  // Moses)" on the A page). Insert the missing tag before splitting.
  const normalized = section.replace(
    /(<\/ul\s*>)(\s*)([A-Za-z][^<\n]*)/gi,
    (_m, close, gap, text) => `${close}${gap}<p>${text}`,
  )

  // Split on <p> and drop the <h2>/back-link preamble that precedes the first one.
  const blocks = normalized.split(/<p\s*>/i).slice(1)

  for (const block of blocks) {
    // "Back to the top" links and stray markup produce empty blocks.
    if (/back to the top/i.test(block) && !/<a\s+href="ch/i.test(block)) continue

    const entry = parseEntry(block, letter, pageName)
    if (entry) entries.push(entry)
  }

  return entries
}

function parseEntry(block, letter, pageName) {
  // The head is everything before the first <ul>; sub-entries live inside it.
  const ulAt = block.search(/<ul\s*>/i)
  const head = ulAt === -1 ? block : block.slice(0, ulAt)
  const body = ulAt === -1 ? '' : block.slice(ulAt)

  // The title is the leading plain text, up to the first anchor or <br>.
  const titleEnd = head.search(/<a\s+href|<br\s*\/?>/i)
  const rawTitle = titleEnd === -1 ? head : head.slice(0, titleEnd)
  const cleanedTitle = stripTags(rawTitle).replace(/[;:,\s]+$/, '')
  if (!cleanedTitle) return null

  const { title, cross_refs } = splitCrossRefs(cleanedTitle)
  if (!title) return null

  // A few entries put the parenthetical after the references rather than after
  // the title ("Conception 7:189; 53:46 (see also Drop, created from)").
  const trailing = /\((?:see\s+also|see)\s+([^)]*)\)/i.exec(stripTags(head))
  if (trailing) {
    for (const ref of trailing[1].split(/,|\band\b|&/).map(collapse)) {
      if (ref && !cross_refs.includes(ref)) cross_refs.push(ref)
    }
  }

  const context = `${pageName} / ${title}`

  // Refs attached directly to the entry (before any sub-entry list).
  const refs = parseRefs(head, context)

  // Sub-entries: unclosed <li> items. Split on <li> and cut each at the
  // first list-closing tag so trailing </ul></ul> noise is discarded.
  const subentries = []
  if (body) {
    const items = body.split(/<li\s*>/i).slice(1)
    for (const item of items) {
      const text = item.split(/<\/ul\s*>/i)[0]
      const labelEnd = text.search(/<a\s+href|<br\s*\/?>/i)
      const rawLabel = labelEnd === -1 ? text : text.slice(0, labelEnd)
      const label = stripTags(rawLabel).replace(/[;:,\s]+$/, '')
      const subRefs = parseRefs(text, `${context} / ${label || '(unlabelled)'}`)
      if (!label && subRefs.length === 0) continue
      subentries.push({ label, refs: subRefs })
    }
  }

  // An entry can legitimately end up with nothing attached when every printed
  // reference failed validation (the source has a handful of broken ones). Keep
  // the title so the entry is still findable, and say so.
  if (refs.length === 0 && subentries.length === 0 && cross_refs.length === 0) {
    anomaly('empty-entry', context, 'kept with no refs, sub-entries or cross-references')
  }

  return {
    letter,
    title,
    sort_key: sortKey(title),
    slug: slugify(title),
    cross_refs,
    refs,
    subentries,
  }
}

// ── Main ────────────────────────────────────────────────────────────────────

async function fetchPages() {
  await mkdir(RAW_DIR, { recursive: true })
  for (const page of PAGES) {
    const url = `${SOURCE_BASE}/qindex_${page}.html`
    const res = await fetch(url)
    if (!res.ok) throw new Error(`${url} -> HTTP ${res.status}`)
    // The pages declare charset=iso-8859-1.
    const buf = Buffer.from(await res.arrayBuffer())
    await writeFile(path.join(RAW_DIR, `qindex_${page}.html`), buf)
    console.log(`fetched ${url} (${buf.length} bytes)`)
  }
}

async function main() {
  if (process.argv.includes('--fetch')) await fetchPages()

  const decoder = new TextDecoder('iso-8859-1')
  const entries = []

  for (const page of PAGES) {
    const file = path.join(RAW_DIR, `qindex_${page}.html`)
    const html = decoder.decode(await readFile(file))
    const parsed = parsePage(html, `qindex_${page}`)
    console.log(`qindex_${page}: ${parsed.length} entries`)
    for (const e of parsed) entries.push(e)
  }

  // Stable order: letter, then sort key, then title.
  entries.sort(
    (a, b) =>
      a.letter.localeCompare(b.letter) ||
      a.sort_key.localeCompare(b.sort_key) ||
      a.title.localeCompare(b.title),
  )
  // The source splits a few long entries (Moses, Satan's) across two <p> blocks.
  // They share a sort key, which would collide on the (lang, sort_key) unique
  // index, so fold the later block into the earlier one.
  const bySortKey = new Map()
  const merged = []
  for (const e of entries) {
    const first = bySortKey.get(e.sort_key)
    if (!first) {
      bySortKey.set(e.sort_key, e)
      merged.push(e)
      continue
    }
    anomaly('merged-duplicate', e.title, `folded a second block into the first`)
    first.refs.push(...e.refs)
    first.subentries.push(...e.subentries)
    for (const ref of e.cross_refs) {
      if (!first.cross_refs.includes(ref)) first.cross_refs.push(ref)
    }
  }
  entries.length = 0
  entries.push(...merged)

  // Positions are assigned after merging so they stay contiguous.
  entries.forEach((e, i) => {
    e.position = i
    e.refs.forEach((r, j) => (r.position = j))
    e.subentries.forEach((sub, j) => {
      sub.position = j
      sub.refs.forEach((r, k) => (r.position = k))
    })
  })

  const subCount = entries.reduce((n, e) => n + e.subentries.length, 0)
  const refCount = entries.reduce(
    (n, e) => n + e.refs.length + e.subentries.reduce((m, s) => m + s.refs.length, 0),
    0,
  )
  const crossCount = entries.reduce((n, e) => n + e.cross_refs.length, 0)

  const payload = {
    metadata: {
      lang: 'en',
      source: `${SOURCE_BASE}/qindex_{${PAGES.join(',')}}.html`,
      source_title: 'Index to: Quran, The Final Testament — by Dr. Rashad Khalifa',
      retrieved: '2026-08-19',
      generator: 'scripts/topic_index/build.mjs',
      note: 'Verse numbers are 1-based, as printed. verses.verse_index in Postgres is 0-based.',
    },
    entries,
  }

  // One entry per line: compact enough to embed, still reviewable in a diff.
  const json =
    '{\n "metadata": ' +
    JSON.stringify(payload.metadata, null, 2).replace(/\n/g, '\n ') +
    ',\n "entries": [\n' +
    payload.entries.map((e) => '  ' + JSON.stringify(e)).join(',\n') +
    '\n ]\n}\n'
  await writeFile(OUT_FILE, json, 'utf8')

  console.log('')
  console.log(`entries         ${entries.length}`)
  console.log(`sub-entries     ${subCount}`)
  console.log(`verse refs      ${refCount}`)
  console.log(`cross-refs      ${crossCount}`)
  console.log(`letters         ${[...new Set(entries.map((e) => e.letter))].join('')}`)
  console.log(`anomalies       ${anomalies.length}`)
  console.log(`wrote           ${path.relative(process.cwd(), OUT_FILE)}`)

  if (anomalies.length > 0) {
    console.log('')
    console.log('Anomalies (source data quirks, not dropped silently):')
    const byKind = new Map()
    for (const a of anomalies) {
      if (!byKind.has(a.kind)) byKind.set(a.kind, [])
      byKind.get(a.kind).push(a)
    }
    for (const [kind, list] of byKind) {
      console.log(`  ${kind} (${list.length})`)
      for (const a of list.slice(0, 12)) console.log(`    ${a.context}: ${a.detail}`)
      if (list.length > 12) console.log(`    ... ${list.length - 12} more`)
    }
  }
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
