const state = {
  user: null,
  ws: null,
  posts: [],
  pendingPosts: [],
  categories: [],
  currentPost: null,
  currentComments: [],
  chats: [],
  activeChat: null,
  messages: [],
  unread: {},
  lastPing: null,
  loadingMessages: false,
  hasMoreMessages: true,
  isTyping: false,
  typingTimeout: null,
};

const views = {
  auth: document.getElementById("auth-view"),
  feed: document.getElementById("feed-view"),
  post: document.getElementById("post-view"),
  create: document.getElementById("create-view"),
  chat: document.getElementById("chat-view"),
};

const chatSidebar = document.getElementById("chat-sidebar");
const chatListEl = document.getElementById("chat-list");
const postsEl = document.getElementById("posts");
const postDetailEl = document.getElementById("post-detail");
const commentsEl = document.getElementById("comments");
const messagesEl = document.getElementById("messages");
const chatHeaderEl = document.getElementById("chat-header");

const categoryFilter = document.getElementById("category-filter");
const typeFilter = document.getElementById("type-filter");
const categorySelect = document.getElementById("category-select");
const bannerEl = document.getElementById("new-posts-banner");
const bannerCountEl = document.getElementById("new-posts-count");
const showNewPostsBtn = document.getElementById("show-new-posts");
const typingIndicator = document.getElementById("typing-indicator");
const messageInput = document.querySelector("#message-form input");
const chatTitleEl = document.getElementById("chat-title");
const chatUnreadEl = document.getElementById("chat-unread");
const clearUnreadBtn = document.getElementById("clear-unread");

function showView(name) {
  Object.values(views).forEach(v => v.classList.remove("active"));
  views[name].classList.add("active");
  if (name !== "chat") {
    setTypingIndicator(false);
  }
}

function formatDate(value) {
  const date = new Date(value);
  return date.toLocaleString("ru-RU", { dateStyle: "medium", timeStyle: "short" });
}

function toTimestamp(value) {
  if (!value) return 0;
  const ts = Date.parse(value);
  return Number.isNaN(ts) ? 0 : ts;
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function getUserLikeValue(value) {
  if (typeof value === "number") return value;
  if (value && typeof value === "object") {
    if (Object.prototype.hasOwnProperty.call(value, "Valid") && value.Valid === false) return 0;
    if (Object.prototype.hasOwnProperty.call(value, "Int64")) return Number(value.Int64) || 0;
  }
  return 0;
}

function renderReactionControls(targetType, targetId, likes, dislikes, userLikeRaw) {
  const userLike = getUserLikeValue(userLikeRaw);
  const likeClass = userLike === 1 ? "reaction-btn active" : "reaction-btn";
  const dislikeClass = userLike === -1 ? "reaction-btn active" : "reaction-btn";
  return `
    <div class="reactions">
      <button class="${likeClass}" data-target-type="${targetType}" data-target-id="${targetId}" data-current-like="${userLike}" data-like-value="1" type="button">👍 ${likes}</button>
      <button class="${dislikeClass}" data-target-type="${targetType}" data-target-id="${targetId}" data-current-like="${userLike}" data-like-value="-1" type="button">👎 ${dislikes}</button>
    </div>
  `;
}

function setUserLikeValue(entity, value) {
  if (!entity) return;
  if (entity.user_like && typeof entity.user_like === "object" && Object.prototype.hasOwnProperty.call(entity.user_like, "Int64")) {
    entity.user_like.Int64 = value;
    entity.user_like.Valid = value !== 0;
  } else {
    entity.user_like = value;
  }
}

function applyReactionToEntity(entity, nextValue) {
  if (!entity) return;
  const prevValue = getUserLikeValue(entity.user_like);
  if (prevValue === nextValue) return;

  if (prevValue === 1) entity.likes = Math.max(0, (entity.likes || 0) - 1);
  if (prevValue === -1) entity.dislikes = Math.max(0, (entity.dislikes || 0) - 1);
  if (nextValue === 1) entity.likes = (entity.likes || 0) + 1;
  if (nextValue === -1) entity.dislikes = (entity.dislikes || 0) + 1;

  setUserLikeValue(entity, nextValue);
}

function saveReactionSnapshot(entity) {
  return {
    entity,
    likes: entity.likes || 0,
    dislikes: entity.dislikes || 0,
    userLike: getUserLikeValue(entity.user_like),
  };
}

function restoreReactionSnapshot(snapshot) {
  const { entity, likes, dislikes, userLike } = snapshot;
  entity.likes = likes;
  entity.dislikes = dislikes;
  setUserLikeValue(entity, userLike);
}

function findCommentByID(comments, targetID) {
  for (const comment of comments) {
    if (comment.id === targetID) return comment;
    const child = findCommentByID(comment.children || [], targetID);
    if (child) return child;
  }
  return null;
}

function reRenderReactions() {
  if (views.feed.classList.contains("active")) {
    renderPosts();
  }
  if (state.currentPost && views.post.classList.contains("active")) {
    renderPostDetail(state.currentPost);
    renderComments(state.currentComments || []);
  }
}

function showBanner() {
  if (!state.pendingPosts.length) return;
  bannerCountEl.textContent = state.pendingPosts.length;
  bannerEl.classList.remove("hidden");
}

function flushPendingPosts() {
  if (!state.pendingPosts.length) return;
  state.posts = [...state.pendingPosts, ...state.posts];
  state.pendingPosts = [];
  bannerEl.classList.add("hidden");
  renderPosts();
}

function setTypingIndicator(visible) {
  if (!typingIndicator) return;
  typingIndicator.classList.toggle("hidden", !visible);
}

function updateUnreadHeader() {
  const total = Object.values(state.unread).reduce((sum, val) => sum + val, 0);
  if (!chatUnreadEl) return;
  if (total > 0) {
    chatUnreadEl.textContent = total;
    chatUnreadEl.classList.remove("hidden");
    if (clearUnreadBtn) clearUnreadBtn.disabled = false;
  } else {
    chatUnreadEl.classList.add("hidden");
    if (clearUnreadBtn) clearUnreadBtn.disabled = true;
  }
}
async function apiFetch(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || "Ошибка запроса");
  }
  return data;
}

async function init() {
  await loadMe();
  bindUI();
  if (state.user) {
    await bootApp();
  } else {
    showAuth();
  }
}

async function loadMe() {
  const data = await apiFetch("/api/me");
  if (Object.prototype.hasOwnProperty.call(data, "user")) {
    state.user = data.user;
  } else {
    state.user = data;
  }
}

function showAuth() {
  document.getElementById("auth-area").style.display = "none";
  chatSidebar.style.display = "none";
  showView("auth");
}

async function bootApp() {
  document.getElementById("auth-area").style.display = "flex";
  chatSidebar.style.display = "block";
  document.getElementById("user-label").textContent = state.user.username;
  await Promise.all([loadCategories(), loadPosts(), loadChats()]);
  connectWS();
  showView("feed");
}

function bindUI() {
  document.querySelectorAll(".nav-btn").forEach(btn => {
    btn.addEventListener("click", () => {
      const view = btn.dataset.view;
      showView(view);
      if (view === "feed") {
        loadPosts();
        showBanner();
      }
    });
  });

  document.getElementById("logout-btn").addEventListener("click", async () => {
    await apiFetch("/api/logout", { method: "POST" });
    state.user = null;
    if (state.ws) state.ws.close();
    showAuth();
  });

  document.getElementById("login-form").addEventListener("submit", async e => {
    e.preventDefault();
    const form = e.target;
    const payload = Object.fromEntries(new FormData(form));
    try {
      const user = await apiFetch("/api/login", { method: "POST", body: JSON.stringify(payload) });
      state.user = user;
      form.reset();
      document.getElementById("login-error").textContent = "";
      await bootApp();
    } catch (err) {
      document.getElementById("login-error").textContent = err.message;
    }
  });

  document.getElementById("register-form").addEventListener("submit", async e => {
    e.preventDefault();
    const form = e.target;
    const payload = Object.fromEntries(new FormData(form));
    payload.age = Number(payload.age);
    try {
      const user = await apiFetch("/api/register", { method: "POST", body: JSON.stringify(payload) });
      state.user = user;
      form.reset();
      document.getElementById("register-error").textContent = "";
      await bootApp();
    } catch (err) {
      document.getElementById("register-error").textContent = err.message;
    }
  });

  document.getElementById("post-form").addEventListener("submit", async e => {
    e.preventDefault();
    const form = e.target;
    const data = Object.fromEntries(new FormData(form));
    const selected = Array.from(categorySelect.selectedOptions).map(o => Number(o.value));
    try {
      const post = await apiFetch("/api/posts", {
        method: "POST",
        body: JSON.stringify({ title: data.title, body: data.body, category_ids: selected }),
      });
      form.reset();
      document.getElementById("post-error").textContent = "";
      state.posts.unshift(post);
      renderPosts();
      showPost(post.id);
    } catch (err) {
      document.getElementById("post-error").textContent = err.message;
    }
  });

  document.getElementById("comment-form").addEventListener("submit", async e => {
    e.preventDefault();
    if (!state.currentPost) return;
    const form = e.target;
    const data = Object.fromEntries(new FormData(form));
    const payload = {
      post_id: state.currentPost.id,
      body: data.body,
      parent_id: data.parent_id ? Number(data.parent_id) : null,
    };
    try {
      await apiFetch("/api/comments", { method: "POST", body: JSON.stringify(payload) });
      form.reset();
      await showPost(state.currentPost.id);
    } catch (err) {
      alert(err.message);
    }
  });

  document.getElementById("message-form").addEventListener("submit", e => {
    e.preventDefault();
    if (!state.activeChat || !state.ws) return;
    const input = e.target.body;
    const body = input.value.trim();
    if (!body) return;
    state.ws.send(JSON.stringify({ type: "send_message", data: { to_user_id: state.activeChat.id, body } }));
    input.value = "";
  });
  messageInput.addEventListener("input", () => {
    if (!state.activeChat || !state.ws) return;
    if (!state.isTyping) {
      state.ws.send(JSON.stringify({ type: "typing", data: { to_user_id: state.activeChat.id, typing: true } }));
      state.isTyping = true;
    }
    clearTimeout(state.typingTimeout);
    state.typingTimeout = setTimeout(() => {
      state.isTyping = false;
      state.ws.send(JSON.stringify({ type: "typing", data: { to_user_id: state.activeChat.id, typing: false } }));
    }, 800);
  });

  document.getElementById("refresh-posts").addEventListener("click", loadPosts);
  showNewPostsBtn.addEventListener("click", flushPendingPosts);
  if (clearUnreadBtn) {
    clearUnreadBtn.addEventListener("click", () => {
      state.unread = {};
      renderChatList();
      updateUnreadHeader();
    });
  }
  categoryFilter.addEventListener("change", loadPosts);
  typeFilter.addEventListener("change", loadPosts);

  document.body.addEventListener("click", async e => {
    const btn = e.target.closest(".reaction-btn");
    if (!btn) return;

    const targetType = btn.dataset.targetType;
    const targetID = Number(btn.dataset.targetId);
    const current = Number(btn.dataset.currentLike || 0);
    const requested = Number(btn.dataset.likeValue);
    const value = current === requested ? 0 : requested;

    if (!targetID || (targetType !== "post" && targetType !== "comment")) return;

    const snapshots = [];
    if (targetType === "post") {
      const postInFeed = state.posts.find(p => p.id === targetID);
      if (postInFeed) snapshots.push(saveReactionSnapshot(postInFeed));
      if (state.currentPost && state.currentPost.id === targetID && state.currentPost !== postInFeed) {
        snapshots.push(saveReactionSnapshot(state.currentPost));
      }
      if (postInFeed) applyReactionToEntity(postInFeed, value);
      if (state.currentPost && state.currentPost.id === targetID) applyReactionToEntity(state.currentPost, value);
    } else {
      const comment = findCommentByID(state.currentComments || [], targetID);
      if (comment) {
        snapshots.push(saveReactionSnapshot(comment));
        applyReactionToEntity(comment, value);
      }
    }
    reRenderReactions();

    try {
      await apiFetch("/api/likes", {
        method: "POST",
        body: JSON.stringify({ target_id: targetID, target_type: targetType, value }),
      });
    } catch (err) {
      snapshots.forEach(restoreReactionSnapshot);
      reRenderReactions();
      alert(err.message);
    }
  });
}

async function loadCategories() {
  const data = await apiFetch("/api/categories");
  state.categories = data.items || [];
  renderCategories();
}

function renderCategories() {
  categoryFilter.innerHTML = "<option value=\"\">Все категории</option>";
  categorySelect.innerHTML = "";
  state.categories.forEach(cat => {
    const opt = document.createElement("option");
    opt.value = cat.id;
    opt.textContent = cat.name;
    categoryFilter.appendChild(opt.cloneNode(true));
    categorySelect.appendChild(opt);
  });
}

async function loadPosts() {
  const params = new URLSearchParams();
  if (categoryFilter.value) params.set("category", categoryFilter.value);
  if (typeFilter.value) params.set("filter", typeFilter.value);
  const data = await apiFetch(`/api/posts?${params.toString()}`);
  state.posts = data.items || [];
  state.pendingPosts = [];
  bannerEl.classList.add("hidden");
  renderPosts();
}

function renderPosts() {
  postsEl.innerHTML = "";
  state.posts.forEach(post => {
    const card = document.createElement("article");
    card.className = "post";
    card.innerHTML = `
      <h3>${escapeHtml(post.title)}</h3>
      <div class="meta">${post.username} • ${formatDate(post.created_at)}</div>
      <div>${escapeHtml(post.body)}</div>
      <div class="tags">${(post.categories || []).map(c => `<span class="tag">${escapeHtml(c.name)}</span>`).join("")}</div>
      ${renderReactionControls("post", post.id, post.likes, post.dislikes, post.user_like)}
      <button class="ghost">Открыть</button>
    `;
    card.querySelector("button").addEventListener("click", () => showPost(post.id));
    postsEl.appendChild(card);
  });
}

async function showPost(id) {
  const data = await apiFetch(`/api/posts/${id}`);
  state.currentPost = data.post;
  state.currentComments = data.comments || [];
  renderPostDetail(data.post);
  renderComments(state.currentComments);
  showView("post");
}

function renderPostDetail(post) {
  postDetailEl.innerHTML = `
    <article class="post">
      <h2>${escapeHtml(post.title)}</h2>
      <div class="meta">${post.username} • ${formatDate(post.created_at)}</div>
      <p>${escapeHtml(post.body)}</p>
      <div class="tags">${(post.categories || []).map(c => `<span class="tag">${escapeHtml(c.name)}</span>`).join("")}</div>
      ${renderReactionControls("post", post.id, post.likes, post.dislikes, post.user_like)}
    </article>
  `;
}

function renderComments(comments) {
  commentsEl.innerHTML = "";
  comments.forEach(c => commentsEl.appendChild(renderComment(c, 0)));
}

function renderComment(comment, depth) {
  const el = document.createElement("div");
  el.className = "comment";
  el.style.marginLeft = `${depth * 16}px`;
  el.innerHTML = `
    <div class="meta">${escapeHtml(comment.username)} • ${formatDate(comment.created_at)}</div>
    <div>${escapeHtml(comment.body)}</div>
    ${renderReactionControls("comment", comment.id, comment.likes, comment.dislikes, comment.user_like)}
    <button class="ghost">Ответить</button>
  `;
  el.querySelector("button").addEventListener("click", () => {
    const form = document.getElementById("comment-form");
    form.parent_id.value = comment.id;
    form.body.focus();
  });
  if (comment.children) {
    comment.children.forEach(child => {
      el.appendChild(renderComment(child, depth + 1));
    });
  }
  return el;
}

async function loadChats() {
  const data = await apiFetch("/api/chats");
  state.chats = data.items || [];
  renderChatList();
  updateUnreadHeader();
}

function renderChatList() {
  chatListEl.innerHTML = "";
  state.chats.forEach(chat => {
    const item = document.createElement("div");
    const unread = state.unread[chat.id] || 0;
    const isActive = state.activeChat && state.activeChat.id === chat.id;
    const ping = state.lastPing === chat.id && !isActive;
    item.className = "chat-item" + (isActive ? " active" : "") + (ping ? " ping" : "");
    item.innerHTML = `
      <div>
        <div>${escapeHtml(chat.username)}</div>
        <div class="status ${chat.online ? "online" : ""}">${chat.online ? "online" : "offline"}</div>
      </div>
      <div class="status">${chat.last_time ? formatDate(chat.last_time) : ""}</div>
      ${unread ? `<span class="badge">${unread}</span>` : ""}
    `;
    item.addEventListener("click", () => openChat(chat));
    chatListEl.appendChild(item);
  });
}

async function openChat(chat) {
  state.activeChat = chat;
  state.messages = [];
  state.hasMoreMessages = true;
  state.unread[chat.id] = 0;
  state.lastPing = null;
  setTypingIndicator(false);
  messagesEl.innerHTML = "";
  chatTitleEl.textContent = escapeHtml(chat.username);
  await loadMoreMessages();
  renderChatList();
  updateUnreadHeader();
  showView("chat");
}

const handleScroll = throttle(() => {
  if (!state.activeChat || state.loadingMessages || !state.hasMoreMessages) return;
  if (messagesEl.scrollTop === 0) {
    loadMoreMessages();
  }
}, 400);

messagesEl.addEventListener("scroll", handleScroll);

async function loadMoreMessages() {
  if (!state.activeChat) return;
  state.loadingMessages = true;
  const beforeId = state.messages.length ? state.messages[0].id : 0;
  const params = new URLSearchParams({ user_id: state.activeChat.id, limit: "10" });
  if (beforeId) params.set("before_id", beforeId);
  const data = await apiFetch(`/api/messages?${params.toString()}`);
  const newMessages = data.items || [];
  if (newMessages.length === 0) {
    state.hasMoreMessages = false;
  } else {
    const prevHeight = messagesEl.scrollHeight;
    state.messages = [...newMessages.reverse(), ...state.messages];
    renderMessages();
    requestAnimationFrame(() => {
      if (beforeId === 0) {
        messagesEl.scrollTop = messagesEl.scrollHeight;
      } else {
        messagesEl.scrollTop = messagesEl.scrollHeight - prevHeight;
      }
    });
  }
  state.loadingMessages = false;
}

function renderMessages() {
  messagesEl.innerHTML = "";
  state.messages.forEach(msg => {
    const el = document.createElement("div");
    el.className = "message";
    const isMine = msg.sender_id === state.user.id;
    const name = isMine ? escapeHtml(state.user.username) : escapeHtml(state.activeChat.username);
    el.innerHTML = `
      <div class="meta">${name} • ${formatDate(msg.created_at)}</div>
      <div>${escapeHtml(msg.body)}</div>
    `;
    messagesEl.appendChild(el);
  });
}

function connectWS() {
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  state.ws = new WebSocket(`${protocol}://${location.host}/ws`);
  state.ws.onmessage = event => {
    const payload = JSON.parse(event.data);
    if (payload.type === "post_created") {
      state.pendingPosts.unshift(payload.data);
      if (views.feed.classList.contains("active")) {
        showBanner();
      }
    }
    if (payload.type === "comment_created") {
      if (state.currentPost && payload.data.post_id === state.currentPost.id) {
        showPost(state.currentPost.id);
      }
    }
    if (payload.type === "pm_message") {
      handleIncomingMessage(payload.data);
    }
    if (payload.type === "presence") {
      applyPresence(payload.data.users || []);
    }
    if (payload.type === "typing") {
      const data = payload.data || {};
      if (state.activeChat && state.activeChat.id === data.from_user_id) {
        setTypingIndicator(Boolean(data.typing));
      }
    }
  };
}

function handleIncomingMessage(msg) {
  const otherId = msg.sender_id === state.user.id ? msg.receiver_id : msg.sender_id;
  const chat = state.chats.find(c => c.id === otherId);
  if (chat) {
    chat.last_time = msg.created_at;
    reorderChats();
  }
  if (!state.activeChat || state.activeChat.id !== otherId) {
    state.unread[otherId] = (state.unread[otherId] || 0) + 1;
    state.lastPing = otherId;
    renderChatList();
    updateUnreadHeader();
    return;
  }
  const atBottom = messagesEl.scrollHeight - messagesEl.scrollTop - messagesEl.clientHeight < 40;
  state.messages.push(msg);
  renderMessages();
  if (atBottom) {
    requestAnimationFrame(() => {
      messagesEl.scrollTop = messagesEl.scrollHeight;
    });
  }
}

function applyPresence(ids) {
  state.chats.forEach(chat => {
    chat.online = ids.includes(chat.id);
  });
  renderChatList();
}

function reorderChats() {
  const withTime = state.chats.filter(c => c.last_time);
  const withoutTime = state.chats.filter(c => !c.last_time);
  withTime.sort((a, b) => toTimestamp(b.last_time) - toTimestamp(a.last_time));
  withoutTime.sort((a, b) => a.username.localeCompare(b.username));
  state.chats = [...withTime, ...withoutTime];
  renderChatList();
}

function throttle(fn, wait) {
  let last = 0;
  let timeout = null;
  return (...args) => {
    const now = Date.now();
    if (now - last >= wait) {
      last = now;
      fn(...args);
    } else if (!timeout) {
      timeout = setTimeout(() => {
        last = Date.now();
        timeout = null;
        fn(...args);
      }, wait - (now - last));
    }
  };
}

init().catch(err => {
  console.error(err);
});
























