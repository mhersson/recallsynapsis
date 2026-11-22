// Sets the look before the first paint; src/theme/theme.ts owns it afterwards.
;(function () {
  function read(key) {
    try {
      return localStorage.getItem(key)
    } catch {
      return null
    }
  }
  var theme = read('theme')
  if (theme !== 'dark' && theme !== 'light') {
    theme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  var root = document.documentElement
  root.dataset.theme = theme
  root.dataset.palette = read('palette') === 'catppuccin' ? 'catppuccin' : 'copper'
})()
