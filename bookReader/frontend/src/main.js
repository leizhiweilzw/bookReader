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
let textAlign = 'left';       // 'left' | 'center' | 'right'
let saveTimer = null;
let sidebarView = 'chapters'; // 'chapters' | 'library'

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
        <div class="reader-layout">
            <aside class="sidebar" id="sidebar">
                <div class="sidebar-header">
                    <span class="sidebar-book-title" id="sidebarBookTitle" title="${escapeHtml(currentBook.title)}">${escapeHtml(currentBook.title)}</span>
                </div>
                <div class="sidebar-toolbar">
                    <button class="sidebar-tab ${sidebarView === 'chapters' ? 'active' : ''}" data-view="chapters" title="章节目录">目录</button>
                    <button class="sidebar-tab ${sidebarView === 'library' ? 'active' : ''}" data-view="library" title="书库">书库</button>
                    <button class="sidebar-open-btn" id="sidebarOpenBtn" title="打开 TXT 文件">+ 打开</button>
                </div>
                <div class="sidebar-list" id="sidebarList">
                    <div class="empty">加载中...</div>
                </div>
            </aside>
            <div class="reader">
                <div class="reader-header">
                    <div class="left">
                        <button class="tool-btn" id="backBtn" title="返回书库首页">← 首页</button>
                        <button class="tool-btn" id="prevBtn" title="上一章">上一章</button>
                    </div>
                    <div class="chapter-title" id="chapterTitle"></div>
                    <div class="right">
                        <select class="chapter-select" id="chapterSelect" title="跳转到章节"></select>
                        <div class="align-group" id="alignGroup" title="对齐方式">
                            <button class="tool-btn align-btn ${textAlign === 'left' ? 'active' : ''}" data-align="left" title="居左"><span style="text-align:left">左</span></button>
                            <button class="tool-btn align-btn ${textAlign === 'center' ? 'active' : ''}" data-align="center" title="居中"><span style="text-align:center">中</span></button>
                            <button class="tool-btn align-btn ${textAlign === 'right' ? 'active' : ''}" data-align="right" title="居右"><span style="text-align:right">右</span></button>
                        </div>
                        <div class="font-group">
                            <button class="tool-btn" id="fontDec" title="缩小字号">A-</button>
                            <button class="tool-btn" id="fontInc" title="放大字号">A+</button>
                        </div>
                        <button class="tool-btn" id="nextBtn" title="下一章">下一章</button>
                    </div>
                </div>
                <div class="progress-bar"><div class="fill" id="progressFill"></div></div>
                <div class="content-wrapper" id="contentWrapper">
                    <div class="chapter-text align-${textAlign}" id="chapterText" style="font-size: ${fontSize}px;"></div>
                </div>
                <div class="reader-footer" id="readerFooter"></div>
            </div>
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

    // Wire reader events
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

    // Alignment buttons
    document.querySelectorAll('.align-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            textAlign = btn.getAttribute('data-align');
            applyTextAlign();
            // Update active state on buttons
            document.querySelectorAll('.align-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
        });
    });

    // Scroll: update progress bar + debounced save
    const wrapper = document.getElementById('contentWrapper');
    wrapper.addEventListener('scroll', onScroll);

    // Sidebar events
    document.getElementById('sidebarOpenBtn').addEventListener('click', handleOpenFile);
    document.querySelectorAll('.sidebar-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            sidebarView = tab.getAttribute('data-view');
            renderReader();
        });
    });
    loadSidebar();
}

// Render the sidebar content based on the current view (chapters or library).
async function loadSidebar() {
    const listEl = document.getElementById('sidebarList');
    if (!listEl) return;

    if (sidebarView === 'library') {
        await renderLibrarySidebar(listEl);
    } else {
        renderChapterSidebar(listEl);
    }
}

// Chapter directory view.
function renderChapterSidebar(listEl) {
    if (!currentBook || !currentBook.chapters) {
        listEl.innerHTML = '<div class="empty">暂无章节</div>';
        return;
    }

    const readCount = currentBook.chapters.filter(c => c.isRead).length;
    const total = currentBook.chapters.length;

    listEl.innerHTML = `
        <div class="sidebar-stats">已读 ${readCount} / ${total} 章</div>
        ${currentBook.chapters.map(c => {
            const isCurrent = c.index === currentChapterIndex;
            let cls = 'chapter-item';
            if (isCurrent) cls += ' current';
            else if (c.isRead) cls += ' read';
            else cls += ' unread';
            return `
                <div class="${cls}" data-index="${c.index}" title="${escapeHtml(c.title)}">
                    <span class="chapter-mark">${c.isRead ? '✓' : (isCurrent ? '▶' : c.index + 1)}</span>
                    <span class="chapter-name">${escapeHtml(c.title)}</span>
                </div>
            `;
        }).join('')}
    `;

    listEl.querySelectorAll('.chapter-item').forEach(item => {
        item.addEventListener('click', () => {
            const idx = parseInt(item.getAttribute('data-index'), 10);
            if (idx !== currentChapterIndex) {
                currentChapterIndex = idx;
                loadChapter(currentChapterIndex, 0);
            }
        });
    });
}

// Library (book list) view inside the sidebar.
async function renderLibrarySidebar(listEl) {
    const recents = await ListRecent().catch(() => []);
    const currentId = currentBook ? currentBook.id : null;

    if (recents.length === 0) {
        listEl.innerHTML = '<div class="empty">书库为空，点击右上角「打开」添加 TXT 小说</div>';
        return;
    }

    listEl.innerHTML = recents.map(r => `
        <div class="sidebar-item ${r.bookId === currentId ? 'active' : ''}" data-id="${r.bookId}">
            <div class="sidebar-item-info">
                <div class="sidebar-item-title">${escapeHtml(r.bookTitle)}</div>
                <div class="sidebar-item-meta">
                    第 ${r.chapterIndex + 1} 章 · ${r.scrollPercent.toFixed(0)}% · ${formatTime(r.updatedAt)}
                </div>
            </div>
            <button class="sidebar-item-remove" data-id="${r.bookId}" title="从书库移除">✕</button>
        </div>
    `).join('');

    listEl.querySelectorAll('.sidebar-item').forEach(item => {
        item.addEventListener('click', (e) => {
            if (e.target.classList.contains('sidebar-item-remove')) return;
            const id = item.getAttribute('data-id');
            if (id === currentId) {
                sidebarView = 'chapters';
                renderReader();
                return;
            }
            handleOpenRecent(id);
        });
    });

    listEl.querySelectorAll('.sidebar-item-remove').forEach(btn => {
        btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const id = btn.getAttribute('data-id');
            await RemoveRecent(id).catch(() => {});
            loadSidebar();
        });
    });
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
        // GetChapter marks the chapter as read on the backend; sync local state.
        if (currentBook.chapters[index]) {
            currentBook.chapters[index].isRead = true;
        }
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
    // Refresh sidebar to update current/read highlighting.
    if (sidebarView === 'chapters') {
        loadSidebar();
    }
}

function onScroll() {
    updateProgressBar();
    scheduleSave();
}

// Apply the current text alignment to the chapter content element.
// For center/right alignment we drop the first-line indent so it looks right.
function applyTextAlign() {
    const el = document.getElementById('chapterText');
    if (!el) return;
    el.classList.remove('align-left', 'align-center', 'align-right');
    el.classList.add('align-' + textAlign);
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
        // Refresh the sidebar so the active book's chapter/percent stay current.
        loadSidebar();
    } catch (err) {
        console.error('save progress failed', err);
    }
}

// registerBookInLibrary ensures an opened book is recorded in the library
// even if the reader hasn't scrolled yet.
async function registerBookInLibrary(book) {
    if (!book) return;
    if (book.progress) return; // already has a saved record
    try {
        await SaveProgress(book.id, 0, 0);
    } catch (err) {
        console.error('register book failed', err);
    }
}

// ===== Actions =====
async function handleOpenFile() {
    try {
        const result = await OpenFile();
        if (result) {
            currentBook = result;
            await registerBookInLibrary(result);
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
