(function (root, factory) {
    var api = factory();
    if (typeof module === 'object' && module.exports) module.exports = api;
    else root.ReleaseDeletions = api;
})(typeof window !== 'undefined' ? window : this, function () {
    'use strict';
    function createState() { return { operations: {}, followed: {} }; }
    function follow(state, id) { state.followed[id] = true; }
    function description(op) {
        if (op.status === 'completed') return { type: 'success', message: 'Release deleted' };
        if (op.status === 'failed') return { type: 'error', message: 'Release deletion is incomplete. Check its status and retry.' };
        return { type: 'info', message: 'Release deletion accepted; cleanup is pending' };
    }
    function apply(state, op) {
        if (!op || !op.id) return null;
        var previous = state.operations[op.id];
        if (previous && Number(previous.version) >= Number(op.version)) return null;
        state.operations[op.id] = op;
        var changed = !previous || previous.status !== op.status;
        return { operation: op, notification: state.followed[op.id] && changed ? description(op) : null };
    }
    function attach(notify, updateEpisode) {
        var state = createState();
        var targets = {};
        try { targets = JSON.parse(sessionStorage.getItem('release-deletion-targets') || '{}'); } catch (e) {}
        function remember(key) { if (!key) return; targets[key] = true; try { sessionStorage.setItem('release-deletion-targets', JSON.stringify(targets)); } catch (e) {} }
        function key(op) { return op.episode_id ? 'episode:' + op.episode_id : 'hash:' + op.hash; }
        function cards(op) {
            var selector = op.episode_id ? '[data-episode-id="' + Number(op.episode_id) + '"]' : '';
            if (op.hash && /^[a-zA-Z0-9]+$/.test(op.hash)) selector += (selector ? ',' : '') + '[data-download-hash="' + op.hash + '"]';
            return selector ? document.querySelectorAll(selector) : [];
        }
        function render(op) {
            cards(op).forEach(function (card) {
                var existing = card.querySelector('.deletion-state');
                var active = op.status !== 'completed';
                card.querySelectorAll('button,select,input,[hx-delete],[hx-post]').forEach(function (control) {
                    if (control.classList.contains('deletion-retry')) return;
                    if (active && !control.disabled) { control.dataset.deletionDisabled = 'true'; control.disabled = true; }
                    if (!active && control.dataset.deletionDisabled) { control.disabled = false; delete control.dataset.deletionDisabled; }
                });
                card.dataset.deletionPending = active ? 'true' : 'false';
                if (!active) { if (existing) existing.remove(); return; }
                if (!existing) { existing = document.createElement('div'); existing.className = 'deletion-state'; existing.setAttribute('role', 'status'); card.appendChild(existing); }
                existing.textContent = (op.status === 'failed' ? 'Deletion incomplete' : 'Deletion pending') + ' · ' + op.stage;
                if (op.status === 'failed') {
                    var retry = document.createElement('button'); retry.type = 'button'; retry.className = 'deletion-retry'; retry.textContent = 'Retry deletion';
                    retry.addEventListener('click', function (event) {
                        event.stopPropagation(); retry.disabled = true; follow(state, op.id); remember(key(op));
                        fetch('/api/v1/release-deletions/' + encodeURIComponent(op.id) + '/retry', { method: 'POST' }).then(function (response) { return response.json(); }).then(consumeResponse).catch(function () { retry.disabled = false; notify('Unable to retry deletion', 'error'); });
                    }); existing.appendChild(retry);
                }
            });
        }
        function consume(op, followed) {
            if (!op || !op.id) return;
            if (followed || targets[key(op)] || targets['hash:' + op.hash]) follow(state, op.id);
            var result = apply(state, op); if (!result) return;
            render(op);
            if (op.status === 'completed' && op.episode_id) updateEpisode(op.episode_id, op.intention === 'blacklist' ? 'wanted' : op.intention, '');
            if (result.notification) notify(result.notification.message, result.notification.type);
        }
        function consumeResponse(response) {
            var data = response.data; if (Array.isArray(data)) data = data[0];
            if (response.operation || (data && data.operation)) { consume(response.operation || data.operation, true); return true; }
            if (response.results) { var found = false; response.results.forEach(function (result) { if (result.operation) { consume(result.operation, true); found = true; } }); return found; }
            return false;
        }
        function refresh() {
            return fetch('/api/v1/release-deletions').then(function (response) { if (!response.ok) throw new Error('query failed'); return response.json(); }).then(function (data) {
                // Render older operations first so the newest request for a target wins.
                (data.operations || []).slice().reverse().forEach(function (op) { consume(op, false); });
            }).catch(function () {});
        }
        function reapply() { Object.values(state.operations).sort(function (a, b) { return a.created_at - b.created_at; }).forEach(render); }
        document.body.addEventListener('htmx:beforeRequest', function (event) {
            var elt = event.detail.elt || event.target;
            var card = elt.closest('[data-episode-id],[data-download-hash]');
            if (card) remember(card.dataset.episodeId ? 'episode:' + card.dataset.episodeId : 'hash:' + card.dataset.downloadHash);
        });
        document.body.addEventListener('htmx:afterRequest', function (event) { try { consumeResponse(JSON.parse(event.detail.xhr.response)); } catch (e) {} });
        document.body.addEventListener('htmx:afterSwap', function () { reapply(); refresh(); });
        refresh();
        return { consume: consume, consumeResponse: consumeResponse, refresh: refresh, reapply: reapply };
    }
    return { createState: createState, follow: follow, apply: apply, description: description, attach: attach };
});
