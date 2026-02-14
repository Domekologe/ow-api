document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('stats-form');
    const tagInput = document.getElementById('tag');
    const platformSelect = document.getElementById('platform');
    const completeToggle = document.getElementById('complete-toggle');

    form.addEventListener('submit', (e) => {
        e.preventDefault();

        // Get values
        let tag = tagInput.value.trim();
        const platform = platformSelect.value;
        const isComplete = completeToggle.checked;
        const type = isComplete ? 'complete' : 'profile';

        // Basic validation
        if (!tag) {
            tagInput.focus();
            shakeElement(tagInput.parentElement);
            return;
        }

        // Format tag: replace # with -
        tag = tag.replace('#', '-');

        // Construct URL
        const url = `/stats/${platform}/${tag}/${type}`;

        // Open in new tab
        window.open(url, '_blank');
    });

    // Add a simple shake animation function for validation feedback
    function shakeElement(element) {
        element.style.animation = 'shake 0.5s cubic-bezier(.36,.07,.19,.97) both';

        // Remove animation after it plays so it can be triggered again
        element.addEventListener('animationend', () => {
            element.style.animation = '';
        }, { once: true });
    }

    // Add shake keyframes to document if not present
    if (!document.getElementById('shake-keyframes')) {
        const style = document.createElement('style');
        style.id = 'shake-keyframes';
        style.textContent = `
            @keyframes shake {
                10%, 90% { transform: translate3d(-1px, 0, 0); }
                20%, 80% { transform: translate3d(2px, 0, 0); }
                30%, 50%, 70% { transform: translate3d(-4px, 0, 0); }
                40%, 60% { transform: translate3d(4px, 0, 0); }
            }
        `;
        document.head.appendChild(style);
    }

    // News System
    const newsContainer = document.getElementById('news-container');

    // Fetch and display news
    function loadNews() {
        fetch('/news')
            .then(res => res.json())
            .then(newsItems => {
                newsContainer.innerHTML = '';
                if (Array.isArray(newsItems) && newsItems.length > 0) {
                    newsItems.forEach(item => {
                        const newsEl = document.createElement('div');
                        newsEl.className = `news-item ${item.type}`;
                        newsEl.innerHTML = `
                            <span class="news-content">${escapeHtml(item.content)}</span>
                        `;
                        newsContainer.appendChild(newsEl);
                    });
                } else {
                    newsContainer.style.display = 'none';
                }
            })
            .catch(err => console.error('Failed to load news:', err));
    }

    loadNews();

    // Helper: Escape HTML
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Fetch GitHub Stars
    const starsElement = document.getElementById('github-stars');
    if (starsElement) {
        fetch('https://api.github.com/repos/Domekologe/ow-api')
            .then(response => {
                if (!response.ok) throw new Error('Network response was not ok');
                return response.json();
            })
            .then(data => {
                if (data.stargazers_count) {
                    // Format number (e.g. 1.2k)
                    const stars = formatNumber(data.stargazers_count);
                    starsElement.textContent = stars;
                    starsElement.style.display = 'inline-block';
                }
            })
            .catch(error => {
                console.error('Error fetching GitHub stars:', error);
                starsElement.style.display = 'none';
            });
    }

    function formatNumber(num) {
        if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'k';
        }
        return num.toString();
    }
});