/**
 * K2A 管理面板 - 前端控制器
 */

class AdminPanel {
    constructor() {
        this.apiBase = '/api/admin';
        this.init();
    }

    async init() {
        // 检查登录状态
        const loggedIn = await this.checkLoginStatus();
        if (loggedIn) {
            this.showAdminView();
            this.refreshTokens();
        } else {
            this.showLoginView();
        }

        this.bindEvents();
    }

    bindEvents() {
        // 登录表单
        document.getElementById('loginForm').addEventListener('submit', (e) => {
            e.preventDefault();
            this.login();
        });

        // 添加 Token 表单
        document.getElementById('addTokenForm').addEventListener('submit', (e) => {
            e.preventDefault();
            this.addToken();
        });

        // 认证类型切换
        document.getElementById('authType').addEventListener('change', (e) => {
            this.toggleIdcFields(e.target.value === 'IdC', '.idc-fields');
        });

        document.getElementById('editAuthType').addEventListener('change', (e) => {
            this.toggleIdcFields(e.target.value === 'IdC', '.edit-idc-fields');
        });

        // 批量添加表单
        document.getElementById('batchAddForm').addEventListener('submit', (e) => {
            e.preventDefault();
            this.batchAddTokens();
        });

        // 上传表单
        document.getElementById('uploadForm').addEventListener('submit', (e) => {
            e.preventDefault();
            this.uploadFile();
        });

        // 修改密码表单
        document.getElementById('changePasswordForm').addEventListener('submit', (e) => {
            e.preventDefault();
            this.changePassword();
        });

        // 编辑 Token 表单
        document.getElementById('editTokenForm').addEventListener('submit', (e) => {
            e.preventDefault();
            this.updateToken();
        });

        // 导入表单
        document.getElementById('importForm').addEventListener('submit', (e) => {
            e.preventDefault();
            this.importConfig();
        });
    }

    toggleIdcFields(show, selector) {
        document.querySelectorAll(selector).forEach(el => {
            el.style.display = show ? 'block' : 'none';
        });
    }

    // === 认证相关 ===

    async checkLoginStatus() {
        try {
            const res = await fetch(`${this.apiBase}/status`);
            const data = await res.json();
            return data.logged_in;
        } catch {
            return false;
        }
    }

    async login() {
        const password = document.getElementById('password').value;
        try {
            const res = await fetch(`${this.apiBase}/login`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password })
            });

            if (res.ok) {
                this.showAdminView();
                this.refreshTokens();
                this.showToast('登录成功', 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '登录失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    async logout() {
        try {
            await fetch(`${this.apiBase}/logout`, { method: 'POST' });
        } catch {}
        this.showLoginView();
        this.showToast('已退出登录', 'info');
    }

    async changePassword() {
        const oldPassword = document.getElementById('oldPassword').value;
        const newPassword = document.getElementById('newPassword').value;
        const confirmPassword = document.getElementById('confirmPassword').value;

        if (newPassword !== confirmPassword) {
            this.showToast('两次输入的密码不一致', 'error');
            return;
        }

        try {
            const res = await fetch(`${this.apiBase}/change-password`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ old_password: oldPassword, new_password: newPassword })
            });

            if (res.ok) {
                closeModal('changePasswordModal');
                document.getElementById('changePasswordForm').reset();
                this.showToast('密码修改成功', 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '修改失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    // === Token 管理 ===

    async refreshTokens() {
        try {
            const res = await fetch(`${this.apiBase}/tokens`);
            if (res.status === 401) {
                this.showLoginView();
                return;
            }

            const data = await res.json();
            this.renderTokens(data.tokens || []);
            this.updateStats(data.stats || {});
        } catch (err) {
            this.showToast('加载失败', 'error');
        }
    }

    renderTokens(tokens) {
        const tbody = document.getElementById('tokenTableBody');

        if (tokens.length === 0) {
            tbody.innerHTML = '<tr><td colspan="8" class="loading">暂无 Token</td></tr>';
            return;
        }

        tbody.innerHTML = tokens.map(token => `
            <tr>
                <td>${token.name || '-'}</td>
                <td>${token.auth || 'Social'}</td>
                <td><span class="token-preview">${token.refreshToken || '-'}</span></td>
                <td>${token.userEmail || '-'}</td>
                <td>${token.remainingUsage >= 0 ? token.remainingUsage : '-'}</td>
                <td>${this.formatDate(token.lastUsed)}</td>
                <td>
                    <span class="status-badge ${token.disabled ? 'status-disabled' : 'status-enabled'}">
                        ${token.disabled ? '禁用' : '启用'}
                    </span>
                </td>
                <td>
                    <div class="action-btns">
                        <button class="btn btn-sm btn-secondary" onclick="adminPanel.editToken('${token.id}')">编辑</button>
                        <button class="btn btn-sm ${token.disabled ? 'btn-success' : 'btn-warning'}"
                                onclick="adminPanel.toggleToken('${token.id}')">
                            ${token.disabled ? '启用' : '禁用'}
                        </button>
                        <button class="btn btn-sm btn-danger" onclick="adminPanel.deleteToken('${token.id}')">删除</button>
                    </div>
                </td>
            </tr>
        `).join('');
    }

    updateStats(stats) {
        document.getElementById('statTotal').textContent = stats.total || 0;
        document.getElementById('statEnabled').textContent = stats.enabled || 0;
        document.getElementById('statDisabled').textContent = stats.disabled || 0;
        document.getElementById('statSocial').textContent = stats.social || 0;
        document.getElementById('statIdc').textContent = stats.idc || 0;
    }

    async addToken() {
        const token = {
            name: document.getElementById('tokenName').value,
            auth: document.getElementById('authType').value,
            refreshToken: document.getElementById('refreshToken').value,
            clientId: document.getElementById('clientId').value,
            clientSecret: document.getElementById('clientSecret').value
        };

        try {
            const res = await fetch(`${this.apiBase}/tokens`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(token)
            });

            if (res.ok) {
                closeModal('addTokenModal');
                document.getElementById('addTokenForm').reset();
                this.refreshTokens();
                this.showToast('添加成功', 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '添加失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    async editToken(id) {
        try {
            const res = await fetch(`${this.apiBase}/tokens/${id}`);
            if (!res.ok) {
                this.showToast('获取 Token 失败', 'error');
                return;
            }

            const token = await res.json();
            document.getElementById('editTokenId').value = token.id;
            document.getElementById('editTokenName').value = token.name || '';
            document.getElementById('editAuthType').value = token.auth || 'Social';
            document.getElementById('editRefreshToken').value = '';
            document.getElementById('editClientId').value = token.clientId || '';
            document.getElementById('editClientSecret').value = '';

            this.toggleIdcFields(token.auth === 'IdC', '.edit-idc-fields');
            showModal('editTokenModal');
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    async updateToken() {
        const id = document.getElementById('editTokenId').value;
        const token = {
            name: document.getElementById('editTokenName').value,
            auth: document.getElementById('editAuthType').value,
            refreshToken: document.getElementById('editRefreshToken').value,
            clientId: document.getElementById('editClientId').value,
            clientSecret: document.getElementById('editClientSecret').value
        };

        try {
            const res = await fetch(`${this.apiBase}/tokens/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(token)
            });

            if (res.ok) {
                closeModal('editTokenModal');
                this.refreshTokens();
                this.showToast('更新成功', 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '更新失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    async toggleToken(id) {
        try {
            const res = await fetch(`${this.apiBase}/tokens/${id}/toggle`, { method: 'POST' });
            if (res.ok) {
                this.refreshTokens();
                this.showToast('状态已切换', 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '操作失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    async deleteToken(id) {
        if (!confirm('确定要删除这个 Token 吗？')) return;

        try {
            const res = await fetch(`${this.apiBase}/tokens/${id}`, { method: 'DELETE' });
            if (res.ok) {
                this.refreshTokens();
                this.showToast('删除成功', 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '删除失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    async batchAddTokens() {
        const text = document.getElementById('batchText').value;
        const authType = document.getElementById('batchAuthType').value;

        const tokens = this.extractTokensFromText(text, authType);

        if (tokens.length === 0) {
            this.showToast('未能提取到有效的 Token', 'error');
            return;
        }

        try {
            const res = await fetch(`${this.apiBase}/tokens/batch`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ tokens })
            });

            if (res.ok) {
                const data = await res.json();
                closeModal('batchAddModal');
                document.getElementById('batchAddForm').reset();
                document.getElementById('extractPreview').style.display = 'none';
                this.refreshTokens();
                this.showToast(`成功添加 ${data.added} 个 Token`, 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '批量添加失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    // 从文本中提取 Token
    extractTokensFromText(text, defaultAuthType = 'Social') {
        const tokens = [];
        const seen = new Set();

        // 1. 尝试解析为 JSON
        try {
            const parsed = JSON.parse(text);
            if (Array.isArray(parsed)) {
                for (const item of parsed) {
                    if (item.refreshToken && !seen.has(item.refreshToken)) {
                        seen.add(item.refreshToken);
                        tokens.push({
                            auth: item.auth || defaultAuthType,
                            refreshToken: item.refreshToken,
                            clientId: item.clientId || '',
                            clientSecret: item.clientSecret || ''
                        });
                    }
                }
                if (tokens.length > 0) return tokens;
            } else if (parsed.refreshToken) {
                return [{
                    auth: parsed.auth || defaultAuthType,
                    refreshToken: parsed.refreshToken,
                    clientId: parsed.clientId || '',
                    clientSecret: parsed.clientSecret || ''
                }];
            }
        } catch {}

        // 2. 提取 "refreshToken": "xxx" 格式
        const jsonPattern = /"refreshToken"\s*:\s*"([^"]+)"/g;
        let match;
        while ((match = jsonPattern.exec(text)) !== null) {
            const token = match[1];
            if (this.isValidRefreshToken(token) && !seen.has(token)) {
                seen.add(token);
                tokens.push({ auth: defaultAuthType, refreshToken: token });
            }
        }

        // 3. 提取 refreshToken: xxx 格式（无引号）
        const noQuotePattern = /refreshToken[:\s]+([a-zA-Z0-9_-]{20,})/gi;
        while ((match = noQuotePattern.exec(text)) !== null) {
            const token = match[1];
            if (this.isValidRefreshToken(token) && !seen.has(token)) {
                seen.add(token);
                tokens.push({ auth: defaultAuthType, refreshToken: token });
            }
        }

        // 4. 提取以 aor 开头的 Token（Kiro 特征）
        const aorPattern = /\b(aor[A-Za-z0-9_-]{30,})/g;
        while ((match = aorPattern.exec(text)) !== null) {
            const token = match[1];
            if (!seen.has(token)) {
                seen.add(token);
                tokens.push({ auth: defaultAuthType, refreshToken: token });
            }
        }

        // 5. 按行分割，每行作为一个 Token
        if (tokens.length === 0) {
            const lines = text.split(/[\n\r]+/);
            for (const line of lines) {
                const trimmed = line.trim();
                if (this.isValidRefreshToken(trimmed) && !seen.has(trimmed)) {
                    seen.add(trimmed);
                    tokens.push({ auth: defaultAuthType, refreshToken: trimmed });
                }
            }
        }

        return tokens;
    }

    // 验证是否为有效的 refreshToken
    isValidRefreshToken(token) {
        if (!token || typeof token !== 'string') return false;
        // 至少 20 个字符，只包含字母数字和 _-
        return /^[a-zA-Z0-9_-]{20,}$/.test(token);
    }

    // 预览提取结果
    previewExtract() {
        const text = document.getElementById('batchText').value;
        const authType = document.getElementById('batchAuthType').value;
        const tokens = this.extractTokensFromText(text, authType);

        const preview = document.getElementById('extractPreview');
        const result = document.getElementById('extractResult');

        if (tokens.length === 0) {
            result.innerHTML = '<p style="color: #dc3545;">未能提取到有效的 Token</p>';
        } else {
            let html = `<p class="token-count">找到 ${tokens.length} 个 Token：</p>`;
            tokens.slice(0, 10).forEach((t, i) => {
                const preview = t.refreshToken.length > 50
                    ? t.refreshToken.substring(0, 25) + '...' + t.refreshToken.substring(t.refreshToken.length - 10)
                    : t.refreshToken;
                html += `<div class="token-item">${i + 1}. [${t.auth}] ${preview}</div>`;
            });
            if (tokens.length > 10) {
                html += `<p style="color: #666; font-size: 12px;">... 还有 ${tokens.length - 10} 个</p>`;
            }
            result.innerHTML = html;
        }

        preview.style.display = 'block';
    }

    async uploadFile() {
        const fileInput = document.getElementById('uploadFile');
        const file = fileInput.files[0];

        if (!file) {
            this.showToast('请选择文件', 'error');
            return;
        }

        const formData = new FormData();
        formData.append('file', file);

        try {
            const res = await fetch(`${this.apiBase}/tokens/upload`, {
                method: 'POST',
                body: formData
            });

            if (res.ok) {
                const data = await res.json();
                closeModal('uploadModal');
                document.getElementById('uploadForm').reset();
                this.refreshTokens();
                this.showToast(`成功添加 ${data.added} 个 Token`, 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '上传失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    // === 导出/导入 ===

    async exportConfig() {
        try {
            const res = await fetch(`${this.apiBase}/export`);
            if (!res.ok) {
                this.showToast('导出失败', 'error');
                return;
            }

            const data = await res.json();

            // 创建下载
            const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `k2a_config_${new Date().toISOString().slice(0, 10)}.json`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);

            this.showToast(`导出成功，共 ${data.tokensCount} 个 Token`, 'success');
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    async importConfig() {
        const fileInput = document.getElementById('importFile');
        const file = fileInput.files[0];
        const mode = document.getElementById('importMode').value;

        if (!file) {
            this.showToast('请选择文件', 'error');
            return;
        }

        // 替换模式需要确认
        if (mode === 'replace') {
            if (!confirm('⚠️ 替换模式将清空所有现有配置！\n\n确定要继续吗？')) {
                return;
            }
        }

        const formData = new FormData();
        formData.append('file', file);

        try {
            const res = await fetch(`${this.apiBase}/import?mode=${mode}`, {
                method: 'POST',
                body: formData
            });

            if (res.ok) {
                const data = await res.json();
                closeModal('importModal');
                document.getElementById('importForm').reset();
                this.refreshTokens();

                const result = data.result;
                let msg = `导入完成：添加 ${result.tokensAdded} 个`;
                if (result.tokensUpdated > 0) {
                    msg += `，更新 ${result.tokensUpdated} 个`;
                }
                if (result.tokensSkipped > 0) {
                    msg += `，跳过 ${result.tokensSkipped} 个`;
                }
                this.showToast(msg, 'success');
            } else {
                const data = await res.json();
                this.showToast(data.error || '导入失败', 'error');
            }
        } catch (err) {
            this.showToast('网络错误', 'error');
        }
    }

    // === UI 辅助 ===

    showLoginView() {
        document.getElementById('loginView').style.display = 'flex';
        document.getElementById('adminView').style.display = 'none';
        document.getElementById('password').value = '';
    }

    showAdminView() {
        document.getElementById('loginView').style.display = 'none';
        document.getElementById('adminView').style.display = 'block';
    }

    formatDate(dateStr) {
        if (!dateStr) return '-';
        try {
            const date = new Date(dateStr);
            return date.toLocaleString('zh-CN', { hour12: false });
        } catch {
            return '-';
        }
    }

    showToast(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.textContent = message;
        document.body.appendChild(toast);

        setTimeout(() => {
            toast.remove();
        }, 3000);
    }
}

// 全局函数
function showModal(id) {
    document.getElementById(id).style.display = 'flex';
}

function closeModal(id) {
    document.getElementById(id).style.display = 'none';
}

function showAddToken() {
    document.getElementById('addTokenForm').reset();
    adminPanel.toggleIdcFields(false, '.idc-fields');
    showModal('addTokenModal');
}

function showBatchAdd() {
    document.getElementById('batchAddForm').reset();
    document.getElementById('extractPreview').style.display = 'none';
    showModal('batchAddModal');
}

function previewExtract() {
    adminPanel.previewExtract();
}

function showUpload() {
    document.getElementById('uploadForm').reset();
    showModal('uploadModal');
}

function showChangePassword() {
    document.getElementById('changePasswordForm').reset();
    showModal('changePasswordModal');
}

function showImport() {
    document.getElementById('importForm').reset();
    showModal('importModal');
}

function exportConfig() {
    adminPanel.exportConfig();
}

function refreshTokens() {
    adminPanel.refreshTokens();
}

function logout() {
    adminPanel.logout();
}

// 初始化
let adminPanel;
document.addEventListener('DOMContentLoaded', () => {
    adminPanel = new AdminPanel();
});
