// Every colour comes from a token: nothing outside tokens.css writes one out.
const sources = import.meta.glob(
  ['/src/**/*.{css,ts,tsx}', '!/src/theme/tokens.css', '!/src/**/*.test.{ts,tsx}'],
  { query: '?raw', import: 'default', eager: true },
) as Record<string, string>

const named = [
  'aliceblue', 'antiquewhite', 'aqua', 'aquamarine', 'azure', 'beige', 'bisque', 'black',
  'blanchedalmond', 'blue', 'blueviolet', 'brown', 'burlywood', 'cadetblue', 'chartreuse',
  'chocolate', 'coral', 'cornflowerblue', 'cornsilk', 'crimson', 'cyan', 'darkblue', 'darkcyan',
  'darkgoldenrod', 'darkgray', 'darkgreen', 'darkgrey', 'darkkhaki', 'darkmagenta',
  'darkolivegreen', 'darkorange', 'darkorchid', 'darkred', 'darksalmon', 'darkseagreen',
  'darkslateblue', 'darkslategray', 'darkslategrey', 'darkturquoise', 'darkviolet', 'deeppink',
  'deepskyblue', 'dimgray', 'dimgrey', 'dodgerblue', 'firebrick', 'floralwhite', 'forestgreen',
  'fuchsia', 'gainsboro', 'ghostwhite', 'gold', 'goldenrod', 'gray', 'green', 'greenyellow',
  'grey', 'honeydew', 'hotpink', 'indianred', 'indigo', 'ivory', 'khaki', 'lavender',
  'lavenderblush', 'lawngreen', 'lemonchiffon', 'lightblue', 'lightcoral', 'lightcyan',
  'lightgoldenrodyellow', 'lightgray', 'lightgreen', 'lightgrey', 'lightpink', 'lightsalmon',
  'lightseagreen', 'lightskyblue', 'lightslategray', 'lightslategrey', 'lightsteelblue',
  'lightyellow', 'lime', 'limegreen', 'linen', 'magenta', 'maroon', 'mediumaquamarine',
  'mediumblue', 'mediumorchid', 'mediumpurple', 'mediumseagreen', 'mediumslateblue',
  'mediumspringgreen', 'mediumturquoise', 'mediumvioletred', 'midnightblue', 'mintcream',
  'mistyrose', 'moccasin', 'navajowhite', 'navy', 'oldlace', 'olive', 'olivedrab', 'orange',
  'orangered', 'orchid', 'palegoldenrod', 'palegreen', 'paleturquoise', 'palevioletred',
  'papayawhip', 'peachpuff', 'peru', 'pink', 'plum', 'powderblue', 'purple', 'rebeccapurple',
  'red', 'rosybrown', 'royalblue', 'saddlebrown', 'salmon', 'sandybrown', 'seagreen', 'seashell',
  'sienna', 'silver', 'skyblue', 'slateblue', 'slategray', 'slategrey', 'snow', 'springgreen',
  'steelblue', 'tan', 'teal', 'thistle', 'tomato', 'turquoise', 'violet', 'wheat', 'white',
  'whitesmoke', 'yellow', 'yellowgreen',
]

const hex = /#(?:[0-9a-f]{8}|[0-9a-f]{6}|[0-9a-f]{3,4})\b/gi
const fn = /\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\(/g
// Not after a word character, '-', '.' or '#', so --red, .red and #red are not colours.
const name = new RegExp(`(?<![\\w.#-])(?:${named.join('|')})(?![\\w-])`, 'gi')

export function coloursIn(file: string, text: string): string[] {
  let code = text
  if (file.endsWith('.css')) {
    code = code.replace(/\/\*[\s\S]*?\*\//g, '').replace(/"[^"\n]*"|'[^'\n]*'/g, '""')
  }
  const found = [...code.matchAll(hex), ...code.matchAll(fn)]
  if (file.endsWith('.css')) found.push(...code.matchAll(name))
  return found.map((m) => m[0])
}

test('no colour is written out outside the tokens', () => {
  expect(Object.keys(sources).length).toBeGreaterThan(0)
  // Raw reads that come back empty would pass for want of anything to find.
  const filled = (ext: string) =>
    Object.entries(sources).some(([file, text]) => file.endsWith(ext) && text.trim() !== '')
  expect(filled('.css')).toBe(true)
  expect(filled('.tsx')).toBe(true)
  const found = Object.entries(sources).flatMap(([file, text]) =>
    coloursIn(file, text).map((m) => `${file}: ${m}`),
  )
  expect(found).toEqual([])
})

test('the matcher catches each kind of colour and allows token mixes', () => {
  const css = (s: string) => coloursIn('a.css', s)
  expect(css('a { color: #fff }')).toEqual(['#fff'])
  expect(css('a { color: #ffffff80 }')).toEqual(['#ffffff80'])
  for (const f of ['rgb(0 0 0)', 'rgba(0,0,0,.5)', 'hsl(1 2% 3%)', 'hsla(1,2%,3%,1)', 'hwb(1 2% 3%)',
    'lab(1 2 3)', 'lch(1 2 3)', 'oklab(1 2 3)', 'oklch(1 2 3)', 'color(srgb 1 1 1)']) {
    expect(css(`a { color: ${f} }`), f).toHaveLength(1)
  }
  expect(css('a { color: red; border-color: RebeccaPurple }')).toEqual(['red', 'RebeccaPurple'])
  expect(coloursIn('a.tsx', "const c = '#abc'")).toEqual(['#abc'])
  expect(css('a { color: color-mix(in oklab, var(--red) 50%, transparent); border-color: currentColor }')).toEqual([])
  expect(css('.red { color: var(--red) } /* white #fff */ [data-x="red"] { top: 0 }')).toEqual([])
})
