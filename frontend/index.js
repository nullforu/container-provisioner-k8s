const defaultPodSpec = `apiVersion: v1
kind: Pod
metadata:
  name: challenge
spec:
  containers:
    - name: app
      image: nginx:stable
      ports:
        - containerPort: 80
          protocol: TCP
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "100m"
          memory: "128Mi"`

function byId(id) {
    return document.getElementById(id)
}

function apiBase() {
    const el = byId('apiBase')
    const raw = el ? el.value.trim() : ''
    if (raw.length > 0) return raw.replace(/\/+$/, '')
    return window.location.origin
}

function apiKeyEnabled() {
    const el = byId('apiKeyEnabled')
    return el ? el.checked : true
}

function apiKeyValue() {
    const el = byId('apiKeyValue')
    return el ? el.value.trim() : ''
}

function readInt(id) {
    const el = byId(id)
    return Number.parseInt(el ? el.value : '0', 10)
}

function syncStackID(stackID) {
    if (!stackID) return
    const last = byId('lastStackId')
    const input = byId('stackIdInput')
    if (last) last.value = stackID
    if (input) input.value = stackID
}

function showResponse(title, payload, state = 'ok') {
    const el = byId('response')
    if (!el) return
    const ts = new Date().toISOString()
    const body = typeof payload === 'string' ? payload : JSON.stringify(payload, null, 2)
    el.textContent = `[${ts}] ${title}\n\n${body}`
    el.dataset.state = state
}

async function request(method, path, body) {
    const url = `${apiBase()}${path}`
    const options = { method, headers: {} }

    if (apiKeyEnabled()) {
        const key = apiKeyValue()
        if (!key) {
            throw new Error('API key is enabled but empty')
        }
        options.headers['X-API-KEY'] = key
    }

    if (body !== undefined) {
        options.headers['Content-Type'] = 'application/json'
        options.body = JSON.stringify(body)
    }

    const res = await fetch(url, options)
    const raw = await res.text()

    let parsed = raw
    try {
        parsed = raw.length ? JSON.parse(raw) : {}
    } catch (_) {}

    const responsePayload = {
        method,
        url,
        status: res.status,
        ok: res.ok,
        body: parsed,
    }

    if (!res.ok) throw responsePayload
    return responsePayload
}

function setBusy(on) {
    document.body.dataset.busy = on ? 'true' : 'false'
    document.querySelectorAll('button[data-action]').forEach((b) => {
        b.disabled = on
    })
}

async function execute(title, fn) {
    try {
        setBusy(true)
        showResponse(title, { status: 'running' }, 'idle')
        const result = await fn()
        showResponse(title, result, 'ok')
    } catch (err) {
        if (err instanceof Error) {
            showResponse(`${title} (ERROR)`, { error: err.message }, 'error')
            return
        }
        showResponse(`${title} (ERROR)`, err, 'error')
    } finally {
        setBusy(false)
    }
}

function getStackID() {
    const el = byId('stackIdInput')
    const stackID = el ? el.value.trim() : ''
    if (!stackID) throw new Error('stack_id is required')
    return stackID
}

function setActiveTab(tab) {
    document.querySelectorAll('[data-tab]').forEach((t) => {
        const active = t.getAttribute('data-tab') === tab
        t.classList.toggle('active', active)
        t.setAttribute('aria-selected', active ? 'true' : 'false')
    })

    document.querySelectorAll('[data-panel]').forEach((p) => {
        const active = p.getAttribute('data-panel') === tab
        p.classList.toggle('active', active)
    })

    try {
        localStorage.setItem('cpdash.activeTab', tab)
    } catch (_) {}
}

function wireTabs() {
    const tabBar = byId('tabBar')
    if (!tabBar) return

    tabBar.addEventListener('click', (e) => {
        const btn = e.target.closest('[data-tab]')
        if (!btn) return
        setActiveTab(btn.getAttribute('data-tab'))
    })

    tabBar.addEventListener('keydown', (e) => {
        if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
        const tabs = Array.from(tabBar.querySelectorAll('[data-tab]'))
        const current = document.activeElement.closest('[data-tab]')
        const idx = Math.max(0, tabs.indexOf(current))
        const next = e.key === 'ArrowRight' ? idx + 1 : idx - 1
        const target = tabs[(next + tabs.length) % tabs.length]
        target.focus()
        setActiveTab(target.getAttribute('data-tab'))
        e.preventDefault()
    })

    let initial = 'settings'
    try {
        initial = localStorage.getItem('cpdash.activeTab') || initial
    } catch (_) {}
    setActiveTab(initial)
}

function wireActions() {
    document.addEventListener('click', (e) => {
        const btn = e.target.closest('button[data-action]')
        if (!btn) return

        const action = btn.getAttribute('data-action')

        if (action === 'health') execute('GET /healthz', () => request('GET', '/healthz'))
        else if (action === 'listStacks') execute('GET /stacks', () => request('GET', '/stacks'))
        else if (action === 'stats') execute('GET /stats', () => request('GET', '/stats'))
        else if (action === 'create') {
            execute('POST /stacks', async () => {
                const payload = {
                    target_port: readInt('createTargetPort'),
                    pod_spec: byId('createPodSpec')?.value ?? '',
                }
                const result = await request('POST', '/stacks', payload)
                const stackID = result?.body?.stack_id ?? ''
                syncStackID(stackID)
                return result
            })
        } else if (action === 'getStack')
            execute('GET /stacks/{stack_id}', () => request('GET', `/stacks/${getStackID()}`))
        else if (action === 'getStatus')
            execute('GET /stacks/{stack_id}/status', () => request('GET', `/stacks/${getStackID()}/status`))
        else if (action === 'deleteStack')
            execute('DELETE /stacks/{stack_id}', () => request('DELETE', `/stacks/${getStackID()}`))
        else if (action === 'clearResponse') {
            const r = byId('response')
            if (r) {
                r.textContent = 'ready'
                r.dataset.state = 'idle'
            }
        }
    })
}

function boot() {
    const api = byId('apiBase')
    const spec = byId('createPodSpec')
    const resp = byId('response')
    const apiKeyEnabledEl = byId('apiKeyEnabled')
    const apiKeyValueEl = byId('apiKeyValue')

    if (api) api.value = window.location.origin
    if (spec) spec.value = defaultPodSpec
    if (resp) resp.dataset.state = 'idle'

    try {
        if (apiKeyEnabledEl) {
            const savedEnabled = localStorage.getItem('cpdash.apiKeyEnabled')
            apiKeyEnabledEl.checked = savedEnabled === null ? true : savedEnabled === 'true'
            apiKeyEnabledEl.addEventListener('change', () => {
                localStorage.setItem('cpdash.apiKeyEnabled', apiKeyEnabledEl.checked ? 'true' : 'false')
            })
        }
        if (apiKeyValueEl) {
            apiKeyValueEl.value = localStorage.getItem('cpdash.apiKeyValue') || ''
            apiKeyValueEl.addEventListener('input', () => {
                localStorage.setItem('cpdash.apiKeyValue', apiKeyValueEl.value)
            })
        }
    } catch (_) {}

    wireTabs()
    wireActions()
}

window.addEventListener('DOMContentLoaded', boot)
