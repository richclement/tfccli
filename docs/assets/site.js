// Copy to clipboard functionality
document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('.copy-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const target = btn.dataset.copy;
      const text = document.querySelector(target)?.textContent || btn.previousElementSibling?.textContent;
      
      if (!text) return;
      
      try {
        await navigator.clipboard.writeText(text.trim());
        btn.classList.add('copied');
        btn.innerHTML = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
        
        setTimeout(() => {
          btn.classList.remove('copied');
          btn.innerHTML = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>`;
        }, 2000);
      } catch (err) {
        console.error('Failed to copy:', err);
      }
    });
  });
});
