const byID = id => document.getElementById(id);
const signInPanel = byID('signin-panel');
const signInForm = byID('signin-form');
const signInButton = byID('signin-submit');
const signInMessage = byID('signin-message');
const captchaSection = byID('captcha-section');
const captchaAnswer = byID('captcha-answer');
const captchaImage = byID('captcha-image');
const captchaRetry = byID('retry-captcha');
const queryPanel = byID('query-panel');
const queryForm = byID('query-form');
const queryButton = byID('query-submit');
const queryMessage = byID('query-message');
const orgRefs = byID('org-refs');
const rows = byID('rows');
const results = byID('results');
const sourceInfo = byID('source-info');
const nextPage = byID('next-page');
const statuses = new Map([
  ['healthy', '正常'], ['degraded', '降级'], ['down', '不可用'], ['unknown', '未知'],
]);

let accessToken = '';
let captchaID = '';
let captchaReady = false;
let signingIn = false;
let activeRead;
let appliedQuery;
let nextCursor = '';

const deviceBytes = crypto.getRandomValues(new Uint8Array(16));
const deviceID = 'reference-assets-' + Array.from(deviceBytes, value => value.toString(16).padStart(2, '0')).join('');

function request(path, init = {}) {
  return fetch(path, {
    ...init,
    cache: 'no-store',
    credentials: 'omit',
    mode: 'same-origin',
    redirect: 'error',
  });
}

function message(element, text, error = false) {
  element.textContent = text;
  element.dataset.error = String(error);
}

async function loadCaptcha(resetMessage = true) {
  captchaReady = false;
  captchaID = '';
  signInButton.disabled = true;
  captchaRetry.hidden = true;
  captchaSection.hidden = true;
  captchaAnswer.required = false;
  captchaAnswer.value = '';
  captchaImage.removeAttribute('src');
  if (resetMessage) {
    message(signInMessage, '正在检查登录要求…');
  }
  try {
    const res = await request('/api/v1/auth/captcha');
    if (!res.ok) throw new Error();
    const {data} = await res.json();
    if (data?.enabled === true) {
      if (typeof data.captcha_id !== 'string' || !data.captcha_id ||
          typeof data.image !== 'string' || !/^data:image\/svg\+xml;base64,[A-Za-z0-9+/=]+$/.test(data.image)) {
        throw new Error();
      }
      captchaID = data.captcha_id;
      captchaImage.src = data.image;
      captchaAnswer.required = true;
      captchaSection.hidden = false;
    } else if (data?.enabled !== false) {
      throw new Error();
    }
    captchaReady = true;
    if (resetMessage) message(signInMessage, '');
  } catch {
    message(signInMessage, '暂时无法检查登录要求，请重试。', true);
    captchaRetry.hidden = false;
  } finally {
    signInButton.disabled = signingIn || !captchaReady;
  }
}

signInForm.addEventListener('submit', async event => {
  event.preventDefault();
  if (signingIn || !captchaReady) return;
  const employeeNo = byID('employee-no').value.trim();
  if (!employeeNo) {
    message(signInMessage, '请输入工号。', true);
    return;
  }
  signingIn = true;
  signInButton.disabled = true;
  message(signInMessage, '正在验证账号…');
  const password = byID('password');
  const payload = {employee_no: employeeNo, password: password.value, device_id: deviceID};
  if (captchaID) {
    payload.captcha_id = captchaID;
    payload.captcha = captchaAnswer.value.trim();
  }
  password.value = '';
  try {
    const res = await request('/api/v1/auth/login', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = res.status === 429 ? '尝试过于频繁，请稍后重试。' :
        res.status >= 500 ? '登录服务暂不可用，请稍后重试。' : '登录失败，请检查工号、密码和验证码。';
      message(signInMessage, text, true);
      return;
    }
    const {data} = await res.json();
    if (data?.must_change_password === true) {
      message(signInMessage, '此账号需先在烛照主应用修改密码，之后再回来登录。', true);
      return;
    }
    if (typeof data?.access_token !== 'string' || !data.access_token) throw new Error();
    accessToken = data.access_token;
    signInForm.reset();
    captchaImage.removeAttribute('src');
    captchaID = '';
    signInPanel.hidden = true;
    queryPanel.hidden = false;
    message(queryMessage, '选择筛选条件后查询。');
    orgRefs.focus();
  } catch {
    message(signInMessage, '登录服务暂不可用，请重试。', true);
  } finally {
    signingIn = false;
    if (!accessToken) await loadCaptcha(false);
  }
});

function clearSession(text, error = false) {
  activeRead?.abort();
  activeRead = undefined;
  accessToken = '';
  queryForm.reset();
  queryButton.disabled = false;
  queryForm.removeAttribute('aria-busy');
  appliedQuery = undefined;
  clearResults();
  queryPanel.hidden = true;
  signInPanel.hidden = false;
  message(signInMessage, text, error);
  byID('employee-no').focus();
  void loadCaptcha(false);
}

function clearResults() {
  nextCursor = '';
  rows.replaceChildren();
  results.hidden = true;
  sourceInfo.hidden = true;
  sourceInfo.textContent = '';
  nextPage.hidden = true;
}

queryForm.addEventListener('submit', event => {
  event.preventDefault();
  const organizations = [...new Set(orgRefs.value.split(/[,，\s]+/).filter(Boolean))];
  if (organizations.length > 256 || organizations.some(id => !/^[1-9]\d*$/.test(id))) {
    clearResults();
    message(queryMessage, '请输入有效的组织 ID，多个 ID 用逗号或空格分隔。', true);
    return;
  }
  const params = new URLSearchParams({limit: byID('page-size').value});
  for (const id of organizations) params.append('owner_org_refs', id);
  const status = byID('status-filter').value;
  if (status) params.set('status', status);
  appliedQuery = params;
  void readPage(params);
});

nextPage.addEventListener('click', () => {
  if (!appliedQuery || !nextCursor) return;
  const params = new URLSearchParams(appliedQuery);
  params.set('cursor', nextCursor);
  void readPage(params);
});

async function readPage(params) {
  if (!accessToken) return;
  activeRead?.abort();
  const read = new AbortController();
  activeRead = read;
  const timer = setTimeout(() => read.abort(), 8000);
  clearResults();
  queryButton.disabled = true;
  queryForm.setAttribute('aria-busy', 'true');
  message(queryMessage, '正在查询…');
  try {
    const res = await request('/api/v1/extensions/reference-assets?' + params, {
      headers: {Authorization: 'Bearer ' + accessToken},
      signal: read.signal,
    });
    if (activeRead !== read) return;
    if (res.status === 401) {
      clearSession('登录会话已失效，请重新登录。', true);
      return;
    }
    if (!res.ok) {
      message(queryMessage, queryError(res.status), true);
      return;
    }
    const page = await res.json();
    if (activeRead === read) renderPage(page);
  } catch {
    if (activeRead === read) message(queryMessage, '查询暂不可用，请稍后重试。', true);
  } finally {
    clearTimeout(timer);
    if (activeRead === read) {
      activeRead = undefined;
      queryButton.disabled = false;
      queryForm.removeAttribute('aria-busy');
    }
  }
}

function queryError(status) {
  switch (status) {
    case 400: return '筛选条件无法识别，请检查后重试。';
    case 403: return '当前账号没有查询权限，或权限已撤销。';
    case 404: return '参考资产查询当前未启用。';
    case 409: return '分页状态已失效或数据源合同已变化，请重新查询。';
    case 429: return '查询过于频繁，请稍后重试。';
    default: return '数据源暂不可用，请稍后重试。';
  }
}

function formatTime(value) {
  if (typeof value !== 'string') return '无观测';
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? '无观测' : new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium', timeStyle: 'short',
  }).format(date);
}

function renderPage(page) {
  if (!Array.isArray(page?.items) || !Array.isArray(page.warnings) || typeof page.complete !== 'boolean' ||
      page.items.some(item => !item || typeof item !== 'object')) {
    message(queryMessage, '数据源返回了无法识别的结果。', true);
    return;
  }
  for (const item of page.items) {
    const row = document.createElement('tr');
    for (const value of [
      item.device_id, item.name, item.owner_org_ref, item.category, statuses.get(item.status) ?? '未知',
      formatTime(item.observed_at), item.stale === true ? '陈旧' : item.observed_at ? '当前' : '—',
    ]) {
      const cell = document.createElement('td');
      cell.textContent = typeof value === 'string' ? value : '—';
      row.append(cell);
    }
    rows.append(row);
  }
  results.hidden = page.items.length === 0;
  const partial = !page.complete && page.warnings.length > 0;
  const count = page.items.length ? '本页显示 ' + page.items.length + ' 条资产。' : '没有匹配或可见的资产。';
  message(queryMessage, partial ? '数据源仅返回部分结果。' + (page.items.length ? count : '') : count);
  const skipped = page.warnings.includes('source_not_queried');
  if (!skipped && typeof page.source_id === 'string') {
    sourceInfo.textContent = '数据源 ' + page.source_id + ' · 观察时间 ' + formatTime(page.observed_at);
    sourceInfo.hidden = false;
  }
  nextCursor = typeof page.next_cursor === 'string' ? page.next_cursor : '';
  nextPage.hidden = !nextCursor;
}

byID('reload-captcha').addEventListener('click', () => void loadCaptcha());
captchaRetry.addEventListener('click', () => void loadCaptcha());
byID('clear-session').addEventListener('click', () => clearSession('此页面的登录会话已清除。'));
void loadCaptcha();
