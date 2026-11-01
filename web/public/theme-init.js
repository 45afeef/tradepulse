// Apply the saved theme before first paint (no flash). Kept as a file so the production CSP can forbid inline scripts.
try { var t = localStorage.getItem('tp.theme'); if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t; } catch (e) {}
