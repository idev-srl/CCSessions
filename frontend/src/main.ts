import './style.css';
import * as Backend from '../wailsjs/go/main/App';

// Shapes of the Go structs (see internal/ccdata). Times arrive as RFC 3339 strings.
interface Session {
    id: string;
    cwd: string;
    firstEvent: string;
    lastEvent: string;
    modTime: string;
    userMessages: number;
    assistantMessages: number;
    fileSize: number;
    extraSize: number;
    title: string;
    firstPrompt: string;
    preview: string;
    badLines: number;
    active: boolean;
}

interface Directory {
    path: string;
    key: string;
    exists: boolean;
    inClaudeJson: boolean;
    unresolved: boolean;
    sessions: Session[];
    sessionCount: number;
    lastSession: string;
    totalSize: number;
    projectFolder: string;
    hasMemory: boolean;
    active: boolean;
}

interface ScanResult {
    directories: Directory[];
    errors: string[] | null;
    scannedAt: string;
    durationMs: number;
}

interface Override {
    source: string;
    kind: string;
    days: number;
    note: string;
}

interface CleanupInfo {
    settingsPath: string;
    days: number;
    found: boolean;
    error: string;
    defaultDays: number;
    minDays: number;
    maxDays: number;
    lastCleanup: string;
    overrides: Override[];
}

interface CleanupPreview {
    days: number;
    sessions: number;
    transcriptBytes: number;
    extraBytes: number;
    totalSessions: number;
}

interface Environment {
    version: string;
    hasWT: boolean;
    hasVSCode: boolean;
    license: string;
    company: string;
    websiteUrl: string;
    repoUrl: string;
    licenseUrl: string;
    configDir: string;
    claudeJson: string;
}

interface RemovalPlan {
    path: string;
    key: string;
    claudeJsonKeys: string[];
    projectFolder: string;
    hasMemory: boolean;
    sessionFiles: string[];
    sessions: number;
    bytes: number;
    blockers: string[];
}

interface RemovalResult {
    claudeJsonBackup: string;
    removedKeys: string[];
    errors: string[];
}

type SortKey = 'path' | 'last' | 'count' | 'size' | 'state';

const $ = <T extends HTMLElement = HTMLElement>(id: string) => document.getElementById(id) as T;

const state = {
    dirs: [] as Directory[],
    errors: [] as string[],
    selectedKey: '',
    sort: {key: 'last' as SortKey, desc: true},
    search: '',
    hideMissing: false,
    hideEmpty: false,
    visible: [] as Directory[],
};

let env: Environment | null = null;

// ---------- formatting (system locale) ----------

const dateFmt = new Intl.DateTimeFormat(undefined, {
    day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit',
});

function toDate(s: string | undefined): Date | null {
    if (!s) return null;
    const d = new Date(s);
    return isNaN(d.getTime()) || d.getFullYear() < 2000 ? null : d;
}

function fmtDate(s: string | undefined): string {
    const d = toDate(s);
    return d ? dateFmt.format(d) : '—';
}

function fmtSize(n: number): string {
    if (!n) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    while (n >= 1024 && i < units.length - 1) {
        n /= 1024;
        i++;
    }
    return `${n.toLocaleString(undefined, {maximumFractionDigits: i === 0 ? 0 : 1})} ${units[i]}`;
}

function fmtDuration(a: string, b: string): string {
    const s = toDate(a), e = toDate(b);
    if (!s || !e) return '—';
    const min = Math.round((e.getTime() - s.getTime()) / 60000);
    if (min < 1) return '< 1 min';
    if (min < 60) return `${min} min`;
    const h = Math.floor(min / 60);
    if (h < 48) return `${h} h ${min % 60} min`;
    return `${Math.floor(h / 24)} d ${h % 24} h`;
}

function plural(n: number, one: string, many = one + 's'): string {
    return `${n} ${n === 1 ? one : many}`;
}

function esc(s: string): string {
    return s.replace(/[&<>"']/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]!));
}

// ---------- status bar ----------

let flashTimer = 0;

function flash(msg: string, isError = false) {
    const el = $('flash');
    el.textContent = msg;
    el.className = isError ? 'error' : '';
    clearTimeout(flashTimer);
    flashTimer = window.setTimeout(() => (el.textContent = ''), isError ? 10000 : 5000);
}

function renderStatus(res: ScanResult | null) {
    if (res) {
        const sessions = state.dirs.reduce((n, d) => n + d.sessionCount, 0);
        const size = state.dirs.reduce((n, d) => n + d.totalSize, 0);
        const active = state.dirs.filter(d => d.active).length;
        const at = toDate(res.scannedAt);
        $('status').textContent =
            `${plural(state.dirs.length, 'directory', 'directories')} · ${plural(sessions, 'session')} · ${fmtSize(size)}` +
            (active ? ` · ${active} active` : '') +
            ` · scanned in ${res.durationMs} ms` + (at ? ` at ${at.toLocaleTimeString()}` : '');
    }
    const btn = $('errorsBtn');
    btn.hidden = state.errors.length === 0;
    btn.textContent = plural(state.errors.length, 'warning');
    const panel = $('errorsPanel');
    panel.innerHTML = state.errors.map(e => `<div>${esc(e)}</div>`).join('');
    if (state.errors.length === 0) panel.hidden = true;
}

async function call<T>(p: Promise<T>, okMsg?: string): Promise<T | undefined> {
    try {
        const r = await p;
        if (okMsg) flash(okMsg);
        return r;
    } catch (e) {
        flash(`Error: ${e}`, true);
        return undefined;
    }
}

// ---------- modal dialog ----------

function confirmDialog(title: string, bodyHtml: string, okLabel: string, okEnabled = true): Promise<boolean> {
    const backdrop = $('modalBackdrop'), ok = $<HTMLButtonElement>('modalOk'), cancel = $<HTMLButtonElement>('modalCancel');
    $('modalTitle').textContent = title;
    $('modalBody').innerHTML = bodyHtml;
    ok.textContent = okLabel;
    ok.disabled = !okEnabled;
    backdrop.hidden = false;
    hideMenu();
    cancel.focus();
    return new Promise(resolve => {
        const done = (v: boolean) => {
            backdrop.hidden = true;
            ok.onclick = cancel.onclick = null;
            document.removeEventListener('keydown', onKey, true);
            resolve(v);
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                e.stopPropagation();
                done(false);
            }
        };
        ok.onclick = () => done(true);
        cancel.onclick = () => done(false);
        document.addEventListener('keydown', onKey, true);
    });
}

// ---------- scanning ----------

async function scan() {
    const btn = $<HTMLButtonElement>('refresh');
    btn.disabled = true;
    $('status').textContent = 'Scanning…';
    try {
        const res = (await Backend.Scan()) as unknown as ScanResult;
        state.dirs = res.directories ?? [];
        state.errors = res.errors ?? [];
        renderStatus(res);
        renderTable();
        renderDetail();
        if (activeTab === 'settings') await loadSettings();
    } catch (e) {
        state.errors = [`Scan failed: ${e}`];
        renderStatus(null);
    } finally {
        btn.disabled = false;
    }
}

// ---------- directory table ----------

function stateLabel(d: Directory): string {
    if (d.unresolved) return 'unknown path';
    if (!d.exists) return 'missing';
    return d.active ? 'active' : 'exists';
}

function compare(a: Directory, b: Directory, key: SortKey): number {
    switch (key) {
        case 'path':
            return a.path.localeCompare(b.path, undefined, {sensitivity: 'base'});
        case 'count':
            return a.sessionCount - b.sessionCount;
        case 'size':
            return a.totalSize - b.totalSize;
        case 'state':
            return stateLabel(a).localeCompare(stateLabel(b));
        case 'last':
            return (toDate(a.lastSession)?.getTime() ?? 0) - (toDate(b.lastSession)?.getTime() ?? 0);
    }
}

function computeVisible() {
    const q = state.search.trim().toLowerCase();
    const {key, desc} = state.sort;
    state.visible = state.dirs
        .filter(d => !q || d.path.toLowerCase().includes(q))
        .filter(d => !state.hideMissing || d.exists)
        .filter(d => !state.hideEmpty || d.sessionCount > 0)
        .sort((a, b) => {
            // Directories without sessions always go last when sorting by date.
            if (key === 'last') {
                const ea = !toDate(a.lastSession), eb = !toDate(b.lastSession);
                if (ea !== eb) return ea ? 1 : -1;
            }
            const c = compare(a, b, key) || a.path.localeCompare(b.path, undefined, {sensitivity: 'base'});
            return desc ? -c : c;
        });
}

const svg = (body: string, fill = false) =>
    `<svg viewBox="0 0 16 16" ${fill ? 'fill="currentColor"' : 'fill="none" stroke="currentColor" stroke-width="1.3"'} aria-hidden="true">${body}</svg>`;

const icons = {
    folder: svg('<path d="M1.5 4.5v8a1 1 0 0 0 1 1h11a1 1 0 0 0 1-1v-6a1 1 0 0 0-1-1H8L6.5 3.5h-4a1 1 0 0 0-1 1z"/>'),
    terminal: svg('<rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M4 6l2.5 2L4 10M8 10.5h4"/>'),
    code: svg('<path d="M5.5 4.5L2 8l3.5 3.5M10.5 4.5L14 8l-3.5 3.5M9 3l-2 10"/>'),
    copy: svg('<rect x="5" y="5" width="8.5" height="9" rx="1"/><path d="M3 11V3a1 1 0 0 1 1-1h6.5"/>'),
    trash: svg('<path d="M2.5 4h11M6 4V2.5h4V4M4 4l.7 9.5h6.6L12 4M6.8 6.5v5M9.2 6.5v5"/>'),
    more: svg('<circle cx="3.5" cy="8" r="1.3"/><circle cx="8" cy="8" r="1.3"/><circle cx="12.5" cy="8" r="1.3"/>', true),
};

function renderTable() {
    computeVisible();
    const tbody = $('dirTable').querySelector('tbody')!;
    tbody.innerHTML = state.visible.map((d, i) => {
        const disabled = d.exists ? '' : 'disabled';
        const badge = d.unresolved ? 'unknown' : !d.exists ? 'missing' : d.active ? 'active' : 'ok';
        const cls = [d.key === state.selectedKey ? 'selected' : '', d.exists ? '' : 'missing'].join(' ');
        const tip = d.inClaudeJson ? d.path : `${d.path}\n(not listed in .claude.json)`;
        return `<tr data-i="${i}" class="${cls}">
            <td class="path" title="${esc(tip)}">${esc(d.path)}</td>
            <td class="num">${fmtDate(d.lastSession)}</td>
            <td class="num">${d.sessionCount}</td>
            <td class="num" title="${d.totalSize.toLocaleString()} bytes">${fmtSize(d.totalSize)}</td>
            <td><span class="badge ${badge}">${stateLabel(d)}</span></td>
            <td><div class="actions">
                <button class="icon-btn" data-act="explorer" title="Open in Explorer" ${disabled}>${icons.folder}</button>
                <button class="icon-btn" data-act="terminal" title="Open in Terminal" ${disabled}>${icons.terminal}</button>
                ${env?.hasVSCode ? `<button class="icon-btn" data-act="vscode" title="Open in VS Code" ${disabled}>${icons.code}</button>` : ''}
                <button class="icon-btn" data-act="copy" title="Copy path" ${d.unresolved ? 'disabled' : ''}>${icons.copy}</button>
                <button class="icon-btn" data-act="menu" title="More actions">${icons.more}</button>
            </div></td>
        </tr>`;
    }).join('');

    document.querySelectorAll<HTMLElement>('#dirTable th[data-sort]').forEach(th => {
        th.classList.remove('sorted-asc', 'sorted-desc');
        if (th.dataset.sort === state.sort.key) th.classList.add(state.sort.desc ? 'sorted-desc' : 'sorted-asc');
    });

    const empty = $('emptyMsg');
    empty.hidden = state.visible.length > 0;
    empty.textContent = state.dirs.length === 0 ? 'No directories found.' : 'No directory matches the filters.';
}

function selected(): Directory | undefined {
    return state.dirs.find(d => d.key === state.selectedKey);
}

function select(d: Directory | undefined, scroll = false) {
    state.selectedKey = d?.key ?? '';
    document.querySelectorAll('#dirTable tbody tr').forEach(tr => {
        const i = Number((tr as HTMLElement).dataset.i);
        tr.classList.toggle('selected', state.visible[i]?.key === state.selectedKey);
        if (scroll && state.visible[i]?.key === state.selectedKey) tr.scrollIntoView({block: 'nearest'});
    });
    renderDetail();
}

function rowDir(el: EventTarget | null): Directory | undefined {
    const tr = (el as HTMLElement | null)?.closest('tr[data-i]') as HTMLElement | null;
    return tr ? state.visible[Number(tr.dataset.i)] : undefined;
}

// ---------- actions ----------

function openExplorer(d: Directory) {
    if (d.exists) call(Backend.OpenExplorer(d.path));
}

function openTerminal(d: Directory, mode = '', sessionId = '') {
    if (d.exists) call(Backend.OpenTerminal(d.path, mode, sessionId));
}

function copy(text: string, what = 'Path copied') {
    call(Backend.CopyText(text), what);
}

function runAction(act: string, d: Directory, anchor?: HTMLElement) {
    switch (act) {
        case 'explorer':
            return openExplorer(d);
        case 'terminal':
            return openTerminal(d);
        case 'continue':
            return openTerminal(d, 'continue');
        case 'resume':
            return openTerminal(d, 'resume');
        case 'vscode':
            if (d.exists) call(Backend.OpenVSCode(d.path));
            return;
        case 'copy':
            return copy(d.path);
        case 'remove':
            return removeDirectory(d);
        case 'menu':
            if (anchor) {
                const r = anchor.getBoundingClientRect();
                showMenu(d, r.right, r.bottom);
            }
    }
}

async function removeDirectory(d: Directory) {
    const plan = await call(Backend.PlanDirectoryRemoval(d.key) as unknown as Promise<RemovalPlan>);
    if (!plan) return;
    const items: string[] = [];
    if (plan.claudeJsonKeys.length) {
        items.push(`<li>${plural(plan.claudeJsonKeys.length, 'entry', 'entries')} in <code>.claude.json</code>:
            ${plan.claudeJsonKeys.map(k => `<code>${esc(k)}</code>`).join(', ')}.
            <span class="warn-text">Per-project permissions, trust and MCP settings are lost.</span>
            A backup <code>.claude.json.ccsessions-bak-&lt;timestamp&gt;</code> is created first.</li>`);
    }
    if (plan.projectFolder) {
        items.push(`<li>The transcript folder <code>${esc(plan.projectFolder)}</code>` +
            (plan.hasMemory ? ` <span class="warn-text">including the project's auto-memory</span>` : '') + `.</li>`);
    }
    if (plan.sessions) {
        items.push(`<li>${plural(plan.sessions, 'session')} (${fmtSize(plan.bytes)})` +
            (plan.sessionFiles.length ? `, of which ${plan.sessionFiles.length} stored in folders shared with other directories` : '') + `.</li>`);
    }
    const blockers = plan.blockers.length
        ? `<div class="blockers"><strong>Cannot remove right now:</strong><ul>${plan.blockers.map(b => `<li>${esc(b)}</li>`).join('')}</ul>
           Claude Code rewrites <code>.claude.json</code> while it runs, so close every Claude Code session and try again.</div>`
        : '';
    const ok = await confirmDialog(
        'Remove directory',
        `<p>Remove <strong>${esc(plan.path)}</strong> from Claude Code? This deletes:</p><ul>${items.join('')}</ul>
         <p>The directory itself and your files in it are <strong>not</strong> touched. This cannot be undone
         (except for <code>.claude.json</code>, which is backed up).</p>${blockers}`,
        'Remove', plan.blockers.length === 0);
    if (!ok) return;
    const res = await call(Backend.RemoveDirectory(d.key, plan.sessions) as unknown as Promise<RemovalResult>);
    if (!res) return;
    if (res.errors.length) {
        state.errors = [...res.errors];
        flash(`Removed with ${plural(res.errors.length, 'error')} — see warnings`, true);
    } else {
        flash(`Removed ${d.path}` + (res.claudeJsonBackup ? ` · backup: ${res.claudeJsonBackup}` : ''));
    }
    if (state.selectedKey === d.key) state.selectedKey = '';
    await scan();
}

async function deleteSession(d: Directory, s: Session) {
    const ok = await confirmDialog(
        'Delete session',
        `<p>Delete this session of <strong>${esc(d.path)}</strong>?</p>
         <ul>
            <li>${esc(s.title || s.preview || s.id)}</li>
            <li>Last activity ${fmtDate(s.lastEvent)} · ${s.userMessages} / ${s.assistantMessages} messages · ${fmtSize(s.fileSize + s.extraSize)}</li>
            <li>ID <code>${esc(s.id)}</code></li>
         </ul>
         <p>The transcript and its subagent/tool-result folder are deleted. This cannot be undone.</p>`,
        'Delete');
    if (!ok) return;
    if (await call(Backend.DeleteSession(d.key, s.id).then(() => true), 'Session deleted')) await scan();
}

// ---------- context menu ----------

function showMenu(d: Directory, x: number, y: number) {
    const menu = $('menu');
    const dis = d.exists ? '' : 'disabled';
    menu.innerHTML = `
        <button data-act="explorer" ${dis}>${icons.folder} Open in Explorer <span class="key">Double-click</span></button>
        <button data-act="terminal" ${dis}>${icons.terminal} Open in Terminal</button>
        ${env?.hasVSCode ? `<button data-act="vscode" ${dis}>${icons.code} Open in VS Code</button>` : ''}
        <hr>
        <button data-act="continue" ${dis}>${icons.terminal} Terminal + <code>claude --continue</code></button>
        <button data-act="resume" ${dis}>${icons.terminal} Terminal + <code>claude --resume</code></button>
        <hr>
        <button data-act="copy" ${d.unresolved ? 'disabled' : ''}>${icons.copy} Copy path</button>
        <hr>
        <button data-act="remove" class="danger-item">${icons.trash} Remove from Claude Code…</button>`;
    menu.hidden = false;
    const w = menu.offsetWidth, h = menu.offsetHeight;
    menu.style.left = `${Math.max(4, Math.min(x, innerWidth - w - 4))}px`;
    menu.style.top = `${Math.max(4, y + h > innerHeight - 4 ? y - h : y)}px`;
    menu.onclick = e => {
        const b = (e.target as HTMLElement).closest('button');
        if (!b || b.disabled) return;
        hideMenu();
        runAction(b.dataset.act!, d);
    };
}

function hideMenu() {
    $('menu').hidden = true;
}

// ---------- detail panel ----------

function renderDetail() {
    const el = $('detail');
    const d = selected();
    if (!d) {
        el.innerHTML = '<div class="detail-empty">Select a directory to see its sessions.</div>';
        return;
    }
    const dis = d.exists ? '' : 'disabled';
    const head = `<div class="detail-head">
        <span class="title" title="${esc(d.path)}">${esc(d.path)}</span>
        <span class="meta">${plural(d.sessionCount, 'session')} · ${fmtSize(d.totalSize)}${d.hasMemory ? ' · auto-memory' : ''}</span>
        <span class="spacer"></span>
        <button class="btn" data-dact="explorer" ${dis}>Explorer</button>
        <button class="btn" data-dact="terminal" ${dis}>Terminal</button>
        <button class="btn" data-dact="continue" ${dis}>claude --continue</button>
        <button class="btn danger-outline" data-dact="remove">Remove…</button>
    </div>`;
    if (d.sessions.length === 0) {
        el.innerHTML = head + `<div class="detail-empty">No transcripts: this directory is only listed in
            <code>.claude.json</code> (its transcripts were removed by the cleanup).</div>`;
    } else {
        el.innerHTML = head + `<table id="sessTable" class="grid">
            <thead><tr>
                <th>Last activity</th><th class="num">Duration</th><th class="num">Messages</th>
                <th class="num">Size</th><th>Preview</th><th></th>
            </tr></thead>
            <tbody>${d.sessions.map((s, i) => {
            const sub = s.title && s.firstPrompt && s.firstPrompt.toLowerCase() !== s.title.toLowerCase()
                ? `<span class="prev-sub">${esc(s.firstPrompt)}</span>` : '';
            const tip = [`ID: ${s.id}`, `Started: ${fmtDate(s.firstEvent)}`, `Last activity: ${fmtDate(s.lastEvent)}`,
                s.title ? `Title: ${s.title}` : '', s.firstPrompt ? `First prompt: ${s.firstPrompt}` : '',
                s.badLines ? `${plural(s.badLines, 'invalid line')}` : ''].filter(Boolean).join('\n');
            const running = s.active ? '<span class="badge active">running</span> ' : '';
            return `<tr data-s="${i}" title="${esc(tip)}">
                    <td>${fmtDate(s.lastEvent)}</td>
                    <td class="num">${fmtDuration(s.firstEvent, s.lastEvent)}</td>
                    <td class="num" title="user / assistant">${s.userMessages} / ${s.assistantMessages}</td>
                    <td class="num">${fmtSize(s.fileSize + s.extraSize)}</td>
                    <td>${running}${s.title ? `<span class="prev-title">${esc(s.title)}</span>` : esc(s.preview || '—')}${sub}</td>
                    <td><div class="actions">
                        <button class="icon-btn" data-sact="resume" title="Resume this session (claude --resume ${esc(s.id)})" ${dis || (s.active ? 'disabled' : '')}>${icons.terminal}</button>
                        <button class="icon-btn" data-sact="copyid" title="Copy session ID">${icons.copy}</button>
                        <button class="icon-btn danger-icon" data-sact="delete" title="${s.active ? 'Running sessions cannot be deleted' : 'Delete session…'}" ${s.active ? 'disabled' : ''}>${icons.trash}</button>
                    </div></td>
                </tr>`;
        }).join('')}</tbody></table>`;
    }
    el.querySelectorAll<HTMLButtonElement>('[data-dact]').forEach(b =>
        b.addEventListener('click', () => runAction(b.dataset.dact!, d)));
    el.querySelectorAll<HTMLButtonElement>('[data-sact]').forEach(b =>
        b.addEventListener('click', () => {
            const s = d.sessions[Number(b.closest('tr')!.dataset.s)];
            switch (b.dataset.sact) {
                case 'resume':
                    return openTerminal(d, 'resume', s.id);
                case 'copyid':
                    return copy(s.id, 'Session ID copied');
                case 'delete':
                    return deleteSession(d, s);
            }
        }));
}

// ---------- settings tab ----------

let cleanupInfo: CleanupInfo | null = null;
let previewTimer = 0;

async function loadSettings() {
    const info = await call(Backend.GetCleanupInfo() as unknown as Promise<CleanupInfo>);
    if (!info) return;
    cleanupInfo = info;
    $('settingsPath').textContent = info.settingsPath;
    $('currentValue').textContent = info.error
        ? `error: ${info.error}`
        : info.found ? plural(info.days, 'day') : `not set — default (${info.defaultDays} days)`;
    $('lastCleanup').textContent = info.lastCleanup ? fmtDate(info.lastCleanup) : '—';
    const input = $<HTMLInputElement>('daysInput');
    input.min = String(info.minDays);
    input.max = String(info.maxDays);
    input.value = String(info.found && info.days >= info.minDays ? info.days : info.defaultDays);
    renderOverrides(info);
    updatePreview();
}

function renderOverrides(info: CleanupInfo) {
    const kinds: Record<string, string> = {
        managed: 'Managed setting', remote: 'Server-managed setting', project: 'Project setting', env: 'Environment variable',
    };
    $('overrides').innerHTML = info.overrides.map(o => `<div class="warn">
        <strong>${esc(kinds[o.kind] ?? o.kind)}</strong>${o.kind !== 'env' ? `: <code>cleanupPeriodDays = ${o.days}</code>` : ''}<br>
        <code>${esc(o.source)}</code><br>${esc(o.note)}
    </div>`).join('') || '<p class="muted small">No managed, policy or project setting overrides this value.</p>';
}

function readDays(): number | null {
    const input = $<HTMLInputElement>('daysInput');
    const err = $('daysError');
    const v = Number(input.value);
    const min = cleanupInfo?.minDays ?? 1, max = cleanupInfo?.maxDays ?? 36500;
    let msg = '';
    if (input.value.trim() === '' || !Number.isInteger(v)) msg = 'Enter a whole number of days.';
    else if (v < min) msg = `The minimum is ${min}: Claude Code rejects 0 with a validation error.`;
    else if (v > max) msg = `The maximum allowed is ${max}.`;
    err.hidden = !msg;
    err.textContent = msg;
    $<HTMLButtonElement>('saveDays').disabled = !!msg;
    return msg ? null : v;
}

function updatePreview() {
    clearTimeout(previewTimer);
    const days = readDays();
    if (days === null) {
        $('preview').textContent = '';
        return;
    }
    previewTimer = window.setTimeout(async () => {
        const p = await call(Backend.PreviewCleanup(days) as unknown as Promise<CleanupPreview>);
        if (!p || p.days !== Number($<HTMLInputElement>('daysInput').value)) return;
        const total = p.transcriptBytes + p.extraBytes;
        $('preview').innerHTML = p.sessions === 0
            ? `With <strong>${plural(days, 'day')}</strong>, the next Claude Code startup would delete none of the ${plural(p.totalSessions, 'session')}.`
            : `With <strong>${plural(days, 'day')}</strong>, the next Claude Code startup will delete
               <strong>${p.sessions}</strong> of ${plural(p.totalSessions, 'session')} (<strong>${fmtSize(total)}</strong>:
               ${fmtSize(p.transcriptBytes)} of transcripts + ${fmtSize(p.extraBytes)} of related files).`;
    }, 120);
}

async function saveDays() {
    const days = readDays();
    if (days === null) return;
    const managed = cleanupInfo?.overrides.some(o => o.kind === 'managed' || o.kind === 'remote');
    const backup = await call(Backend.SaveCleanupDays(days));
    if (backup === undefined) return;
    flash(`Saved cleanupPeriodDays = ${days}` + (backup ? ` · backup: ${backup}` : '') +
        (managed ? ' · note: a managed setting takes precedence' : ''));
    await loadSettings();
}

// ---------- about tab ----------

function renderAbout() {
    if (!env) return;
    $('appVersion').textContent = env.version;
    $('aboutConfigDir').textContent = env.configDir;
    $('aboutClaudeJson').textContent = env.claudeJson;
}

// ---------- tabs, splitter, events ----------

let activeTab = 'dirs';

function showTab(tab: string) {
    activeTab = tab;
    document.querySelectorAll<HTMLElement>('.tab').forEach(t => t.classList.toggle('active', t.dataset.tab === tab));
    document.querySelectorAll<HTMLElement>('.tab-page').forEach(p => p.classList.toggle('active', p.id === `tab-${tab}`));
    $('dirTools').style.visibility = tab === 'dirs' ? 'visible' : 'hidden';
    if (tab === 'settings') loadSettings();
}

function setupSplitter() {
    const splitter = $('splitter'), top = $('dirTableWrap'), page = $('tab-dirs');
    try {
        const saved = localStorage.getItem('splitRatio');
        if (saved) top.style.flex = `0 0 ${saved}%`;
    } catch { /* storage unavailable */ }
    splitter.addEventListener('pointerdown', e => {
        splitter.setPointerCapture(e.pointerId);
        const rect = page.getBoundingClientRect();
        const move = (ev: PointerEvent) => {
            const pct = Math.min(85, Math.max(15, ((ev.clientY - rect.top) / rect.height) * 100));
            top.style.flex = `0 0 ${pct}%`;
            try {
                localStorage.setItem('splitRatio', pct.toFixed(1));
            } catch { /* ignore */ }
        };
        const up = () => {
            splitter.removeEventListener('pointermove', move);
            splitter.removeEventListener('pointerup', up);
        };
        splitter.addEventListener('pointermove', move);
        splitter.addEventListener('pointerup', up);
    });
}

function loadPrefs() {
    try {
        state.hideMissing = localStorage.getItem('hideMissing') === '1';
        state.hideEmpty = localStorage.getItem('hideEmpty') === '1';
    } catch { /* ignore */ }
    $<HTMLInputElement>('hideMissing').checked = state.hideMissing;
    $<HTMLInputElement>('hideEmpty').checked = state.hideEmpty;
}

function savePref(key: string, v: boolean) {
    try {
        localStorage.setItem(key, v ? '1' : '0');
    } catch { /* ignore */ }
}

async function init() {
    loadPrefs();
    setupSplitter();
    renderDetail();

    document.querySelectorAll<HTMLElement>('.tab').forEach(t => t.addEventListener('click', () => showTab(t.dataset.tab!)));
    $('refresh').addEventListener('click', scan);

    $<HTMLInputElement>('search').addEventListener('input', e => {
        state.search = (e.target as HTMLInputElement).value;
        renderTable();
    });
    $<HTMLInputElement>('hideMissing').addEventListener('change', e => {
        state.hideMissing = (e.target as HTMLInputElement).checked;
        savePref('hideMissing', state.hideMissing);
        renderTable();
    });
    $<HTMLInputElement>('hideEmpty').addEventListener('change', e => {
        state.hideEmpty = (e.target as HTMLInputElement).checked;
        savePref('hideEmpty', state.hideEmpty);
        renderTable();
    });

    document.querySelectorAll<HTMLElement>('#dirTable th[data-sort]').forEach(th => th.addEventListener('click', () => {
        const key = th.dataset.sort as SortKey;
        state.sort = state.sort.key === key ? {key, desc: !state.sort.desc} : {key, desc: key !== 'path' && key !== 'state'};
        renderTable();
    }));

    const tbody = $('dirTable').querySelector('tbody')!;
    tbody.addEventListener('click', e => {
        const d = rowDir(e.target);
        if (!d) return;
        const btn = (e.target as HTMLElement).closest('button[data-act]') as HTMLButtonElement | null;
        if (d.key !== state.selectedKey) select(d);
        if (btn && !btn.disabled) runAction(btn.dataset.act!, d, btn);
    });
    tbody.addEventListener('dblclick', e => {
        if ((e.target as HTMLElement).closest('button')) return;
        const d = rowDir(e.target);
        if (d) openExplorer(d);
    });
    tbody.addEventListener('contextmenu', e => {
        const d = rowDir(e.target);
        if (!d) return;
        e.preventDefault();
        select(d);
        showMenu(d, e.clientX, e.clientY);
    });

    document.querySelectorAll<HTMLAnchorElement>('a[data-url]').forEach(a => a.addEventListener('click', e => {
        e.preventDefault();
        if (!env) return;
        const url = {website: env.websiteUrl, repo: env.repoUrl, license: env.licenseUrl}[a.dataset.url!];
        if (url) call(Backend.OpenURL(url));
    }));

    document.addEventListener('contextmenu', e => e.preventDefault());
    document.addEventListener('mousedown', e => {
        if (!$('menu').contains(e.target as Node)) hideMenu();
        if (!$('errorsPanel').contains(e.target as Node) && e.target !== $('errorsBtn')) $('errorsPanel').hidden = true;
    });
    window.addEventListener('blur', hideMenu);
    $('errorsBtn').addEventListener('click', () => ($('errorsPanel').hidden = !$('errorsPanel').hidden));

    document.addEventListener('keydown', e => {
        if (!$('modalBackdrop').hidden) return;
        if (e.key === 'Escape') {
            hideMenu();
            $('errorsPanel').hidden = true;
            return;
        }
        if (e.key === 'F5') {
            e.preventDefault();
            scan();
            return;
        }
        if (e.ctrlKey && e.key.toLowerCase() === 'f' && activeTab === 'dirs') {
            e.preventDefault();
            $('search').focus();
            return;
        }
        if (activeTab !== 'dirs' || (e.target as HTMLElement).tagName === 'INPUT') return;
        const idx = state.visible.findIndex(d => d.key === state.selectedKey);
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            const n = Math.max(0, Math.min(state.visible.length - 1, idx + (e.key === 'ArrowDown' ? 1 : -1)));
            select(state.visible[n], true);
        } else if (e.key === 'Enter' && idx >= 0) {
            openExplorer(state.visible[idx]);
        } else if (e.key === 'Delete' && idx >= 0) {
            removeDirectory(state.visible[idx]);
        }
    });

    $('daysInput').addEventListener('input', updatePreview);
    $('daysInput').addEventListener('keydown', e => {
        if (e.key === 'Enter') saveDays();
    });
    $('saveDays').addEventListener('click', saveDays);

    env = (await call(Backend.GetEnvironment() as unknown as Promise<Environment>)) ?? null;
    renderAbout();
    await scan();
}

init();
