import './style.css';
import './app.css';

// Safe bridge: the Wails Go bindings are only injected inside the desktop
// WebView window. When running in a plain browser (e.g. during dev inspection),
// we fall back to no-op stubs so the UI still renders.
const bridge = window['go'] && window['go']['main'] && window['go']['main']['App'];

function callGo(method, ...args) {
    if (!bridge || typeof bridge[method] !== 'function') {
        return Promise.reject(new Error('Wails 桥接不可用（请在桌面应用窗口中使用完整功能）'));
    }
    return bridge[method](...args);
}

const OpenFile = () => callGo('OpenFile');
const OpenRecent = (id) => callGo('OpenRecent', id);
const GetChapter = (id, idx) => callGo('GetChapter', id, idx);
const SaveProgress = (id, idx, pct) => callGo('SaveProgress', id, idx, pct);
const ListRecent = () => callGo('ListRecent');
const RemoveRecent = (id) => callGo('RemoveRecent', id);

const app = document.querySelector('#app');

// ===== State =====
let currentBook = null;       // OpenResult
let currentChapterIndex = 0;
let fontSize = 18;
let saveTimer = null;

// ===== Rendering =====
function render() {
    if (currentBook) {
        renderReader();
    } else {
        renderLibrary();
    }
}

// ----- Library screen -----
async function renderLibrary() {
    const recents = await ListRecent().catch(() => []);

    app.innerHTML = `
        <div class="library">
            <h1>小说阅读器</h1>
            <p class="subtitle">支持本地 TXT 小说 · 自动识别章节 · 记录阅读位置</p>
            <button class="open-btn" id="openBtn">打开 TXT 文件</button>

            <div class="recent-section">
                <h2>最近阅读</h2>
                ${recents.length === 0
                    ? '<div class="empty">暂无阅读记录，点击上方按钮打开一本小说吧</div>'
                    : `<ul class="recent-list">
                        ${recents.map(r => `
                            <li class="recent-item" data-id="${r.bookId}">
                                <div class="info">
                                    <div class="title">${escapeHtml(r.bookTitle)}</div>
                                    <div class="meta">
                                        第 ${r.chapterIndex + 1} 章 · ${r.scrollPercent.toFixed(1)}% ·
                                        ${formatTime(r.updatedAt)}
                                    </div>
                                </div>
                                <div class="actions">
                                    <button class="icon-btn remove-btn" data-id="${r.bookId}" title="移除记录">✕</button>
                                </div>
                            </li>
                        `).join('')}
                    </ul>`
                }
            </div>
        </div>
    `;

    document.getElementById('openBtn').addEventListener('click', handleOpenFile);

    document.querySelectorAll('.recent-item').forEach(item => {
        item.addEventListener('click', (e) => {
            if (e.target.classList.contains('remove-btn')) return;
            const id = item.getAttribute('data-id');
            handleOpenRecent(id);
        });
    });

    document.querySelectorAll('.remove-btn').forEach(btn => {
        btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const id = btn.getAttribute('data-id');
            await RemoveRecent(id).catch(() => {});
            renderLibrary();
        });
    });
}

// ----- Reader screen -----
function renderReader() {
    app.innerHTML = `
        <div class="reader">
            <div class="reader-header">
                <div class="left">
                    <button class="tool-btn" id="backBtn" title="返回书库">← 书库</button>
                    <button class="tool-btn" id="prevBtn" title="上一章">上一章</button>
                </div>
                <div class="chapter-title" id="chapterTitle"></div>
                <div class="right">
                    <select class="chapter-select" id="chapterSelect" title="跳转到章节"></select>
                    <div class="font-group">
                        <button class="tool-btn" id="fontDec" title="缩小字号">A-</button>
                        <button class="tool-btn" id="fontInc" title="放大字号">A+</button>
                    </div>
                    <button class="tool-btn" id="nextBtn" title="下一章">下一章</button>
                </div>
            </div>
            <div class="progress-bar"><div class="fill" id="progressFill"></div></div>
            <div class="content-wrapper" id="contentWrapper">
                <div class="chapter-text" id="chapterText" style="font-size: ${fontSize}px;"></div>
            </div>
            <div class="reader-footer" id="readerFooter"></div>
        </div>
        <div class="toast" id="toast"></div>
    `;

    // Populate chapter select
    const select = document.getElementById('chapterSelect');
    currentBook.chapters.forEach((c, i) => {
        const opt = document.createElement('option');
        opt.value = i;
        opt.textContent = `${i + 1}. ${c.title}`;
        select.appendChild(opt);
    });

    // Determine starting chapter: use saved progress if present, otherwise start at 0
    let startIndex = 0;
    let startScroll = 0;
    if (currentBook.progress) {
        startIndex = currentBook.progress.chapterIndex;
        startScroll = currentBook.progress.scrollPercent;
    }

    currentChapterIndex = startIndex;
    loadChapter(currentChapterIndex, startScroll);

    // Wire events
    document.getElementById('backBtn').addEventListener('click', () => {
        flushProgress();
        currentBook = null;
        render();
    });

    document.getElementById('prevBtn').addEventListener('click', () => {
        if (currentChapterIndex > 0) {
            currentChapterIndex--;
            loadChapter(currentChapterIndex, 0);
        }
    });

    document.getElementById('nextBtn').addEventListener('click', () => {
        if (currentChapterIndex < currentBook.chapterCount - 1) {
            currentChapterIndex++;
            loadChapter(currentChapterIndex, 0);
        }
    });

    select.addEventListener('change', (e) => {
        currentChapterIndex = parseInt(e.target.value, 10);
        loadChapter(currentChapterIndex, 0);
    });

    document.getElementById('fontDec').addEventListener('click', () => {
        fontSize = Math.max(12, fontSize - 2);
        document.getElementById('chapterText').style.fontSize = fontSize + 'px';
    });

    document.getElementById('fontInc').addEventListener('click', () => {
        fontSize = Math.min(36, fontSize + 2);
        document.getElementById('chapterText').style.fontSize = fontSize + 'px';
    });

    // Scroll: update progress bar + debounced save
    const wrapper = document.getElementById('contentWrapper');
    wrapper.addEventListener('scroll', onScroll);
}

async function loadChapter(index, scrollPercent) {
    const select = document.getElementById('chapterSelect');
    const titleEl = document.getElementById('chapterTitle');
    const textEl = document.getElementById('chapterText');
    const footer = document.getElementById('readerFooter');
    const prevBtn = document.getElementById('prevBtn');
    const nextBtn = document.getElementById('nextBtn');

    select.value = index;
    const chapter = currentBook.chapters[index];
    titleEl.textContent = chapter.title;
    textEl.textContent = '加载中...';

    prevBtn.disabled = index === 0;
    nextBtn.disabled = index === currentBook.chapterCount - 1;

    try {
        const text = await GetChapter(currentBook.id, index);
        textEl.textContent = text || '(本章无内容)';
    } catch (err) {
        textEl.textContent = '加载失败: ' + err;
    }

    // Restore scroll position after content is painted
    requestAnimationFrame(() => {
        const wrapper = document.getElementById('contentWrapper');
        const maxScroll = wrapper.scrollHeight - wrapper.clientHeight;
        wrapper.scrollTop = (scrollPercent / 100) * maxScroll;
        updateProgressBar();
    });

    footer.textContent = `第 ${index + 1} / ${currentBook.chapterCount} 章`;
    currentChapterIndex = index;
    scheduleSave();
}

function onScroll() {
    updateProgressBar();
    scheduleSave();
}

function updateProgressBar() {
    const wrapper = document.getElementById('contentWrapper');
    const fill = document.getElementById('progressFill');
    if (!wrapper || !fill) return;
    const max = wrapper.scrollHeight - wrapper.clientHeight;
    const pct = max > 0 ? (wrapper.scrollTop / max) * 100 : 0;
    fill.style.width = pct + '%';
}

function currentScrollPercent() {
    const wrapper = document.getElementById('contentWrapper');
    if (!wrapper) return 0;
    const max = wrapper.scrollHeight - wrapper.clientHeight;
    return max > 0 ? (wrapper.scrollTop / max) * 100 : 0;
}

function scheduleSave() {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(flushProgress, 800);
}

async function flushProgress() {
    if (!currentBook) return;
    const pct = currentScrollPercent();
    try {
        await SaveProgress(currentBook.id, currentChapterIndex, pct);
    } catch (err) {
        console.error('save progress failed', err);
    }
}

// ===== Actions =====
async function handleOpenFile() {
    try {
        const result = await OpenFile();
        if (result) {
            currentBook = result;
            render();
        }
    } catch (err) {
        showToast('打开失败: ' + err);
    }
}

async function handleOpenRecent(bookID) {
    try {
        const result = await OpenRecent(bookID);
        if (result) {
            currentBook = result;
            render();
        } else {
            showToast('无法打开该书籍');
        }
    } catch (err) {
        showToast('打开失败: ' + err);
    }
}

function showToast(msg) {
    let toast = document.getElementById('toast');
    if (!toast) {
        toast = document.createElement('div');
        toast.id = 'toast';
        toast.className = 'toast';
        document.body.appendChild(toast);
    }
    toast.textContent = msg;
    toast.classList.add('show');
    setTimeout(() => toast.classList.remove('show'), 2000);
}

// ===== Helpers =====
function escapeHtml(s) {
    const div = document.createElement('div');
    div.textContent = s == null ? '' : String(s);
    return div.innerHTML;
}

function formatTime(ts) {
    if (!ts) return '';
    const d = new Date(ts * 1000);
    const now = new Date();
    const diff = (now - d) / 1000;
    if (diff < 60) return '刚刚';
    if (diff < 3600) return Math.floor(diff / 60) + ' 分钟前';
    if (diff < 86400) return Math.floor(diff / 3600) + ' 小时前';
    if (diff < 7 * 86400) return Math.floor(diff / 86400) + ' 天前';
    return `${d.getMonth() + 1}-${d.getDate()}`;
}

// ===== Init =====
render();
