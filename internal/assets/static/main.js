// User customizable configuration options
const CLIENT_CONFIG = {
    // Timeout in milliseconds to redirect the page after port changes
    portRedirectDelayMs: 1500,
    // Debounce delay in milliseconds for volume adjustments
    volumeDebounceDelayMs: 250,
    // Enable debug logging for WebSockets and settings
    debugLogging: false
};

// ----------------------------------------------------
// Toast Notification System
// ----------------------------------------------------

function showToast(message, type = "info", duration = 5000) {
    var container = document.getElementById("toast-container");
    if (!container) {
        container = document.createElement("div");
        container.id = "toast-container";
        container.className = "toast-container";
        document.body.appendChild(container);
    }

    var toast = document.createElement("div");
    toast.className = "toast toast-" + type;

    var iconMap = { success: "\u2713", error: "\u2715", info: "\u2139" };
    var icon = document.createElement("span");
    icon.className = "toast-icon";
    icon.textContent = iconMap[type] || iconMap.info;

    var text = document.createElement("span");
    text.textContent = message;
    text.style.flex = "1";

    var closeBtn = document.createElement("button");
    closeBtn.textContent = "\u2715";
    closeBtn.className = "toast-close";

    var autoDismiss = 0;

    function dismissToast() {
        clearTimeout(autoDismiss);
        toast.style.animation = "none";
        toast.style.opacity = "0";
        toast.style.transform = "translateY(20px)";
        setTimeout(function() { toast.remove(); }, 300);
    }

    closeBtn.onclick = function(e) {
        e.stopPropagation();
        e.preventDefault();
        dismissToast();
    };

    toast.appendChild(icon);
    toast.appendChild(text);
    toast.appendChild(closeBtn);
    container.appendChild(toast);

    autoDismiss = setTimeout(function() {
        dismissToast();
    }, duration);
}

// ----------------------------------------------------
// Styled Confirm/Prompt Modals (replaces native confirm/prompt)
// ----------------------------------------------------

function showConfirmModal(message) {
    return new Promise(function(resolve) {
        const overlay = document.createElement("div");
        overlay.className = "confirm-modal-overlay";
        const content = document.createElement("div");
        content.className = "confirm-modal-content";
        const msg = document.createElement("p");
        msg.className = "confirm-modal-message";
        msg.textContent = message;
        const btns = document.createElement("div");
        btns.className = "confirm-modal-buttons";
        const cancelBtn = document.createElement("button");
        cancelBtn.className = "confirm-modal-cancel";
        cancelBtn.textContent = "Cancel";
        const okBtn = document.createElement("button");
        okBtn.className = "confirm-modal-ok";
        okBtn.textContent = "OK";
        btns.appendChild(cancelBtn);
        btns.appendChild(okBtn);
        content.appendChild(msg);
        content.appendChild(btns);
        overlay.appendChild(content);
        document.body.appendChild(overlay);
        setModalOpen(true);

        function cleanup(result) {
            overlay.remove();
            setModalOpen(false);
            resolve(result);
        }

        okBtn.addEventListener("click", function() { cleanup(true); });
        cancelBtn.addEventListener("click", function() { cleanup(false); });
        overlay.addEventListener("click", function(e) { if (e.target === overlay) cleanup(false); });
    });
}

function showPromptModal(message, defaultValue) {
    return new Promise(function(resolve) {
        const overlay = document.createElement("div");
        overlay.className = "confirm-modal-overlay";
        const content = document.createElement("div");
        content.className = "confirm-modal-content";
        const msg = document.createElement("p");
        msg.className = "confirm-modal-message";
        msg.textContent = message;
        const input = document.createElement("input");
        input.className = "confirm-modal-input";
        input.type = "text";
        input.value = defaultValue || "";
        input.enterKeyHint = "done";
        const btns = document.createElement("div");
        btns.className = "confirm-modal-buttons";
        const cancelBtn = document.createElement("button");
        cancelBtn.className = "confirm-modal-cancel";
        cancelBtn.textContent = "Cancel";
        const okBtn = document.createElement("button");
        okBtn.className = "confirm-modal-ok";
        okBtn.textContent = "OK";
        btns.appendChild(cancelBtn);
        btns.appendChild(okBtn);
        content.appendChild(msg);
        content.appendChild(input);
        content.appendChild(btns);
        overlay.appendChild(content);
        document.body.appendChild(overlay);
        setModalOpen(true);
        input.focus();

        function cleanup(result) {
            overlay.remove();
            setModalOpen(false);
            resolve(result);
        }

        okBtn.addEventListener("click", function() { cleanup(input.value.trim() || null); });
        cancelBtn.addEventListener("click", function() { cleanup(null); });
        input.addEventListener("keydown", function(e) { if (e.key === "Enter") { e.preventDefault(); cleanup(input.value.trim() || null); } });
        overlay.addEventListener("click", function(e) { if (e.target === overlay) cleanup(null); });
    });
}

// ----------------------------------------------------
// Layout Editor Undo History
// ----------------------------------------------------

let layoutHistory = [];
let historyIndex = -1;
let draggedWidget = null;
let placeholder = null;
let dragGhost = null;
let dragOffsetX = 0;
let dragOffsetY = 0;
let dragPointerId = -1;

function pushLayoutHistory() {
    const page = document.getElementById("page");
    if (!page) return;
    const html = page.innerHTML;
    // Avoid pushing duplicate consecutive states
    if (layoutHistory.length > 0 && layoutHistory[historyIndex] === html) return;
    // Trim any future history if we undid and then made a new change
    if (historyIndex < layoutHistory.length - 1) {
        layoutHistory = layoutHistory.slice(0, historyIndex + 1);
    }
    layoutHistory.push(html);
    historyIndex = layoutHistory.length - 1;
    updateUndoButtonState();
}

function undoLastLayoutChange() {
    if (historyIndex <= 0) return;
    historyIndex--;
    const page = document.getElementById("page");
    if (page && layoutHistory[historyIndex] !== undefined) {
        page.innerHTML = layoutHistory[historyIndex];
        // Re-bind edit mode UI since innerHTML wipe destroys listeners
        enableWidgetsDraggability(false);
        enableWidgetsDraggability(true);
    }
    updateUndoButtonState();
}

function updateUndoButtonState() {
    const btnUndo = document.getElementById("btn-undo-layout");
    if (!btnUndo) return;
    if (historyIndex > 0) {
        btnUndo.style.opacity = "1";
        btnUndo.style.color = "var(--color-primary)";
        btnUndo.style.pointerEvents = "auto";
    } else {
        btnUndo.style.opacity = "0.5";
        btnUndo.style.color = "var(--color-text-subdue)";
        btnUndo.style.pointerEvents = "none";
    }
}

function throttledDebounce(callback, maxDebounceTimes, debounceDelay) {
    let debounceTimeout;
    let timesDebounced = 0;

    return function () {
        if (timesDebounced === maxDebounceTimes) {
            clearTimeout(debounceTimeout);
            timesDebounced = 0;
            callback();
            return;
        }

        clearTimeout(debounceTimeout);
        timesDebounced++;

        debounceTimeout = setTimeout(() => {
            timesDebounced = 0;
            callback();
        }, debounceDelay);
    };
}

async function fetchPageContents(pageSlug) {
    const response = await fetch(`/api/pages/${pageSlug}/content/`);
    if (!response.ok) {
        throw new Error(`Failed to fetch page content: ${response.status}`);
    }
    const content = await response.text();
    return content;
}

function setupCarousels() {
    const carouselElements = document.getElementsByClassName("carousel-container");

    for (let i = 0; i < carouselElements.length; i++) {
        const carousel = carouselElements[i];
        const itemsContainer = carousel.getElementsByClassName("carousel-items-container")[0];
        if (!itemsContainer) continue;

        const determineSideCutoffs = () => {
            if (itemsContainer.scrollLeft != 0) {
                carousel.classList.add("show-left-cutoff");
            } else {
                carousel.classList.remove("show-left-cutoff");
            }

            if (Math.ceil(itemsContainer.scrollLeft) + itemsContainer.clientWidth < itemsContainer.scrollWidth) {
                carousel.classList.add("show-right-cutoff");
            } else {
                carousel.classList.remove("show-right-cutoff");
            }
        };

        const determineSideCutoffsRateLimited = throttledDebounce(determineSideCutoffs, 20, 100);

        itemsContainer.addEventListener("scroll", determineSideCutoffsRateLimited);
        
        // Use ResizeObserver instead of window resize listener to avoid listener leaks on window
        const resizeObserver = new ResizeObserver(() => {
            determineSideCutoffsRateLimited();
        });
        resizeObserver.observe(itemsContainer);

        determineSideCutoffs();
    }
}

const minuteInSeconds = 60;
const hourInSeconds = minuteInSeconds * 60;
const dayInSeconds = hourInSeconds * 24;
const monthInSeconds = dayInSeconds * 30;
const yearInSeconds = monthInSeconds * 12;

function relativeTimeSince(timestamp) {
    const delta = Math.round((Date.now() / 1000) - timestamp);

    if (delta < minuteInSeconds) {
        return "1m";
    }
    if (delta < hourInSeconds) {
        return Math.floor(delta / minuteInSeconds) + "m";
    }
    if (delta < dayInSeconds) {
        return Math.floor(delta / hourInSeconds) + "h";
    }
    if (delta < monthInSeconds) {
        return Math.floor(delta / dayInSeconds) + "d";
    }
    if (delta < yearInSeconds) {
        return Math.floor(delta / monthInSeconds) + "mo";
    }

    return Math.floor(delta / yearInSeconds) + "y";
}

function updateRelativeTimeForElements(elements) {
    for (let i = 0; i < elements.length; i++) {
        const element = elements[i];
        const timestamp = element.dataset.dynamicRelativeTime;

        if (timestamp === undefined)
            continue;

        element.innerText = relativeTimeSince(timestamp);
    }
}

function setupDynamicRelativeTime() {
    const updateElementsAndTimestamp = () => {
        const elements = document.querySelectorAll("[data-dynamic-relative-time]");
        updateRelativeTimeForElements(elements);
        window.lastRelativeUpdateTime = Date.now();
    };

    updateElementsAndTimestamp();

    if (!window.relativeTimeIntervalInitialized) {
        window.relativeTimeIntervalInitialized = true;
        const updateInterval = 60 * 1000;
        window.lastRelativeUpdateTime = Date.now();

        const scheduleRepeatingUpdate = () => setInterval(updateElementsAndTimestamp, updateInterval);

        if (document.hidden === undefined) {
            scheduleRepeatingUpdate();
            return;
        }

        let timeout = scheduleRepeatingUpdate();

        document.addEventListener("visibilitychange", () => {
            if (document.hidden) {
                clearInterval(timeout);
                return;
            }

            const delta = Date.now() - window.lastRelativeUpdateTime;

            if (delta >= updateInterval) {
                updateElementsAndTimestamp();
                timeout = scheduleRepeatingUpdate();
                return;
            }

            timeout = setTimeout(() => {
                updateElementsAndTimestamp();
                timeout = scheduleRepeatingUpdate();
            }, updateInterval - delta);
        });
    }
}

function setupLazyImages() {
    const images = document.querySelectorAll("img[loading=lazy]");

    if (images.length == 0) {
        return;
    }

    function imageFinishedTransition(image) {
        image.classList.add("finished-transition");
    }

    for (let i = 0; i < images.length; i++) {
        const image = images[i];

        if (image.complete) {
            image.classList.add("cached");
            setTimeout(() => imageFinishedTransition(image), 5);
        } else {
            image.addEventListener("load", () => {
                image.classList.add("loaded");
                setTimeout(() => imageFinishedTransition(image), 500);
            });
        }
    }
}

// ----------------------------------------------------
// Spotify Integration Logic
// ----------------------------------------------------

let spotifyInterval;
let lastSpotifyState = null;

// Format milliseconds into M:SS representation
function formatTime(ms) {
    const totalSecs = Math.max(0, Math.floor(ms / 1000));
    const mins = Math.floor(totalSecs / 60);
    const secs = totalSecs % 60;
    return mins + ":" + (secs < 10 ? "0" : "") + secs;
}

// Starts local, fast-ticking progress interval to prevent layout progress bar lagging
function startLocalSpotifyTicker(progressMs, durationMs, isPlaying) {
    clearInterval(spotifyInterval);
    const progressFill = document.getElementById("spotify-progress-fill");
    const timeProgress = document.getElementById("spotify-time-progress");

    if (!progressFill || !timeProgress) return;
    if (!durationMs || durationMs <= 0) return;

    let currentProgress = progressMs;
    
    // Initial display update
    timeProgress.innerText = formatTime(currentProgress);
    const percent = Math.min(100, (currentProgress / durationMs) * 100);
    progressFill.style.transform = "scaleX(" + (percent / 100) + ")";

    if (!isPlaying || currentProgress >= durationMs) return;

    spotifyInterval = setInterval(() => {
        currentProgress += 1000;
        if (currentProgress > durationMs) {
            currentProgress = durationMs;
            clearInterval(spotifyInterval);
        }
        timeProgress.innerText = formatTime(currentProgress);
        const percent = Math.min(100, (currentProgress / durationMs) * 100);
        progressFill.style.transform = "scaleX(" + (percent / 100) + ")";
    }, 1000);
}

// Opens the edit widget modal for the Spotify widget by entering edit mode if needed
function openSpotifyWidgetSettings() {
    const player = document.getElementById("spotify-player");
    if (!player) return;
    // Enter edit mode so dataset attributes are populated
    if (!document.body.classList.contains("layout-edit-mode")) {
        toggleEditMode(true);
    }
    const widget = player.closest(".widget");
    if (!widget) return;
    const col = widget.dataset.originalCol;
    const idx = widget.dataset.originalIdx;
    const nestedIdx = widget.dataset.originalNestedIdx !== undefined ? parseInt(widget.dataset.originalNestedIdx) : undefined;
    if (col !== undefined && idx !== undefined) {
        openEditWidgetModal(col, parseInt(idx), nestedIdx);
    } else {
        showToast("Unable to locate widget.", "error");
    }
}

// Updates the DOM of the Spotify widget with parsed WebSocket updates
function updateSpotifyWidget(data) {
    console.log("[Spotify] updateSpotifyWidget called with:", data);
    const player = document.getElementById("spotify-player");
    if (!player) return;

    const loginPrompt = document.getElementById("spotify-login-prompt");
    const playerContent = document.getElementById("spotify-player-content");

    if (!data || !data.authorized) {
        if (loginPrompt) loginPrompt.style.display = "block";
        if (playerContent) playerContent.style.display = "none";
        clearInterval(spotifyInterval);
        return;
    }

    if (loginPrompt) loginPrompt.style.display = "none";
    if (playerContent) playerContent.style.display = "block";

    const track = data.track;
    const error = data.error;
    console.log("[Spotify] track object:", track, "track.id:", track ? track.id : undefined, "error:", error);

    const activeTrack = document.getElementById("spotify-active-track");
    const idleState = document.getElementById("spotify-idle-state");
    const errorState = document.getElementById("spotify-error-state");
    const iconPlay = document.getElementById("spotify-icon-play");
    const iconPause = document.getElementById("spotify-icon-pause");
    const volumeSlider = document.getElementById("spotify-volume-slider");
    const volumeVal = document.getElementById("spotify-volume-val");

    if (error) {
        console.log("[Spotify] Showing error state:", error);
        if (activeTrack) activeTrack.style.display = "none";
        if (idleState) idleState.style.display = "none";
        if (errorState) {
            errorState.style.display = "block";
            const msgEl = document.getElementById("spotify-error-message");
            if (msgEl) msgEl.innerText = error;
        }
        if (iconPlay) iconPlay.style.display = "block";
        if (iconPause) iconPause.style.display = "none";
        clearInterval(spotifyInterval);
        return;
    }

    if (errorState) errorState.style.display = "none";

    if (!track || !track.id) {
        console.log("[Spotify] Showing idle state (no track or no track.id)");
        if (activeTrack) activeTrack.style.display = "none";
        if (idleState) idleState.style.display = "block";
        if (iconPlay) iconPlay.style.display = "block";
        if (iconPause) iconPause.style.display = "none";
        clearInterval(spotifyInterval);
        return;
    }

    console.log("[Spotify] Showing active track state");
    if (activeTrack) activeTrack.style.display = "block";
    if (idleState) idleState.style.display = "none";

    const albumArt = document.getElementById("spotify-album-art");
    const trackTitle = document.getElementById("spotify-track-title");
    const trackArtist = document.getElementById("spotify-track-artist");
    const trackAlbum = document.getElementById("spotify-track-album");

    if (albumArt) albumArt.src = track.image_url;
    if (trackTitle) trackTitle.innerText = track.title;
    if (trackArtist) trackArtist.innerText = track.artist;
    if (trackAlbum) trackAlbum.innerText = track.album;

    if (track.is_playing) {
        if (iconPlay) iconPlay.style.display = "none";
        if (iconPause) iconPause.style.display = "block";
    } else {
        if (iconPlay) iconPlay.style.display = "block";
        if (iconPause) iconPause.style.display = "none";
    }

    if (volumeSlider) volumeSlider.value = track.volume;
    if (volumeVal) volumeVal.innerText = track.volume + "%";

    const timeDuration = document.getElementById("spotify-time-duration");
    if (timeDuration) {
        timeDuration.innerText = formatTime(track.duration_ms);
    }

    startLocalSpotifyTicker(track.progress_ms, track.duration_ms, track.is_playing);
}

// Configures click and slider input events for the Spotify widget
function setupSpotifyControls() {
    document.addEventListener("click", async (e) => {
        const btnPlayPause = e.target.closest("#spotify-btn-play-pause");
        const btnPrev = e.target.closest("#spotify-btn-prev");
        const btnNext = e.target.closest("#spotify-btn-next");

        try {
            if (btnPlayPause) {
                const iconPlay = document.getElementById("spotify-icon-play");
                const iconPause = document.getElementById("spotify-icon-pause");
                const isPlaying = iconPause && iconPause.style.display !== "none";

                // Optimistically toggle the display state of play/pause SVG icons
                if (isPlaying) {
                    if (iconPlay) iconPlay.style.display = "block";
                    if (iconPause) iconPause.style.display = "none";
                } else {
                    if (iconPlay) iconPlay.style.display = "none";
                    if (iconPause) iconPause.style.display = "block";
                }

                const action = isPlaying ? "pause" : "play";
                const resp = await fetch(`/api/spotify/${action}`, { method: "POST" });
                if (!resp.ok) {
                    console.warn("[Spotify] Control action failed:", resp.status);
                    // Revert the optimistic toggling if the backend action fails
                    if (isPlaying) {
                        if (iconPlay) iconPlay.style.display = "none";
                        if (iconPause) iconPause.style.display = "block";
                    } else {
                        if (iconPlay) iconPlay.style.display = "block";
                        if (iconPause) iconPause.style.display = "none";
                    }
                }
            }
            if (btnPrev) {
                const resp = await fetch("/api/spotify/skip?direction=prev", { method: "POST" });
                if (!resp.ok) console.warn("[Spotify] Skip previous failed:", resp.status);
            }
            if (btnNext) {
                const resp = await fetch("/api/spotify/skip?direction=next", { method: "POST" });
                if (!resp.ok) console.warn("[Spotify] Skip next failed:", resp.status);
            }
        } catch (e) {
            console.warn("[Spotify] Control action error:", e);
        }
    });

    document.addEventListener("input", (e) => {
        const slider = e.target.closest("#spotify-volume-slider");
        if (slider) {
            const volumeVal = document.getElementById("spotify-volume-val");
            if (volumeVal) volumeVal.innerText = slider.value + "%";
            debouncedVolumeSet(slider.value);
        }
    });

    const hint = document.getElementById("spotify-redirect-hint");
    if (hint) {
        const redirectURI = window.location.origin + "/api/spotify/callback";
        hint.textContent = "Redirect URI: " + redirectURI;
    }
}

let volumeDebounceTimeout;
function debouncedVolumeSet(vol) {
    clearTimeout(volumeDebounceTimeout);
    volumeDebounceTimeout = setTimeout(() => {
        fetch(`/api/spotify/volume?volume=${vol}`, { method: "POST" });
    }, CLIENT_CONFIG.volumeDebounceDelayMs);
}

let pageRefreshTimeout;
let ignoreReloadPageUntil = 0;
const RELOAD_PAGE_IGNORE_DURATION_MS = 5000;

function triggerLivePageRefreshDebounced() {
    clearTimeout(pageRefreshTimeout);
    pageRefreshTimeout = setTimeout(async () => {
        await refreshPageContentsLive();
    }, 200);
}

// Establishes a WebSocket connection for real-time status syncing
function setupWebSockets() {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws`);

    ws.onmessage = function (event) {
        try {
            const msg = JSON.parse(event.data);
            if (msg.type === "spotify_update") {
                console.log("[Spotify] WS update raw:", msg.data);
                lastSpotifyState = msg.data;
                // Cache authorization state to prevent flash on reconnect
                if (msg.data && typeof msg.data.authorized === "boolean") {
                    localStorage.setItem("spotify_last_auth", msg.data.authorized ? "true" : "false");
                }
                updateSpotifyWidget(msg.data);
            } else if (msg.type === "reload_page") {
                if (Date.now() > ignoreReloadPageUntil) {
                    triggerLivePageRefreshDebounced();
                }
            }
        } catch (e) {
            console.error("[WS] Error parsing message:", e);
        }
    };

    ws.onclose = function () {
        console.log("[WS] Connection lost. Reconnecting in 5s...");
        setTimeout(setupWebSockets, 5000);
    };
}

// ----------------------------------------------------
// Layout Drag and Drop Editor
// ----------------------------------------------------

let layoutOriginalHTML = "";

function toggleEditMode(active) {
    const body = document.body;
    const btnEdit = document.getElementById("btn-edit-layout");
    const btnAdd = document.getElementById("btn-add-widget");
    const btnSave = document.getElementById("btn-save-layout");
    const btnCancel = document.getElementById("btn-cancel-layout");
    const btnUndo = document.getElementById("btn-undo-layout");
    const btnAddPage = document.getElementById("btn-add-page");
    const selectLayout = document.getElementById("select-page-layout");

    if (active) {
        layoutOriginalHTML = document.getElementById("page").innerHTML;
        layoutHistory = [layoutOriginalHTML];
        historyIndex = 0;
        body.classList.add("layout-edit-mode");
        btnEdit.style.display = "none";
        btnAdd.style.display = "block";
        btnSave.style.display = "block";
        btnCancel.style.display = "block";
        if (btnUndo) {
            btnUndo.style.display = "block";
            updateUndoButtonState();
        }
        if (btnAddPage) btnAddPage.style.display = "inline-block";
        if (selectLayout) {
            selectLayout.style.display = "block";
            // Pre-select current layout based on columns rendered in DOM
            const columns = document.querySelectorAll(".page-column");
            const sizes = [];
            columns.forEach(col => {
                if (col.classList.contains("page-column-full")) {
                    sizes.push("full");
                } else if (col.classList.contains("page-column-small")) {
                    sizes.push("small");
                }
            });
            const layoutKey = sizes.join(",");
            selectLayout.value = layoutKey || "full";
        }
        enableWidgetsDraggability(true);
    } else {
        cancelPointerDrag();
        body.classList.remove("layout-edit-mode");
        btnEdit.style.display = "block";
        btnAdd.style.display = "none";
        btnSave.style.display = "none";
        btnCancel.style.display = "none";
        if (btnUndo) btnUndo.style.display = "none";
        if (btnAddPage) btnAddPage.style.display = "none";
        if (selectLayout) selectLayout.style.display = "none";
        enableWidgetsDraggability(false);
        layoutHistory = [];
        historyIndex = -1;
    }
}

// Configures drag handle and edit/delete actions for both page columns and page header columns, including nested Group widgets
function enableWidgetsDraggability(enabled) {
    const headCol = document.querySelector(".page-column-head");
    const columns = document.querySelectorAll(".page-column");

    // Assemble list of all active drop target columns
    const allCols = [];
    if (headCol) {
        allCols.push({ el: headCol, index: "head" });
    }
    columns.forEach((col, colIdx) => {
        allCols.push({ el: col, index: colIdx });
    });

    const setupWidget = (w, colIdx, wIdx, nestedIdx) => {
        if (w.dataset.originalCol === undefined) {
            w.dataset.originalCol = colIdx;
            w.dataset.originalIdx = wIdx;
            if (nestedIdx !== undefined) {
                w.dataset.originalNestedIdx = nestedIdx;
            }
        }

        if (enabled) {
            w.classList.add("editable");

            if (!w.querySelector(".widget-drag-handle")) {
                const header = w.querySelector(".widget-header");
                if (header) {
                    const handle = document.createElement("div");
                    handle.className = "widget-drag-handle";
                    handle.title = "Drag to reorder";
                    handle.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="10" height="16" viewBox="0 0 10 16" fill="currentColor" style="display:block;"><circle cx="2.5" cy="2.5" r="1.5"/><circle cx="7.5" cy="2.5" r="1.5"/><circle cx="2.5" cy="8" r="1.5"/><circle cx="7.5" cy="8" r="1.5"/><circle cx="2.5" cy="13.5" r="1.5"/><circle cx="7.5" cy="13.5" r="1.5"/></svg>';
                    handle.addEventListener("pointerdown", function(e) { startPointerDrag(e, w); });
                    header.insertBefore(handle, header.firstChild);
                }
            }

            // Inject a button container with Edit + Delete if not already present
            if (!w.querySelector(".widget-edit-actions")) {
                const header = w.querySelector(".widget-header");
                if (header) {
                    // Container to hold edit and delete buttons side-by-side, pushed to the right
                    const actionsDiv = document.createElement("div");
                    actionsDiv.className = "widget-edit-actions";
                    actionsDiv.style.cssText = "margin-left: auto; display: flex; align-items: center; gap: 6px; flex-shrink: 0;";

                    // Edit/pencil button using inline SVG for reliable cross-platform rendering
                    const editBtn = document.createElement("button");
                    editBtn.type = "button";
                    editBtn.className = "widget-edit-btn";
                    editBtn.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/></svg>`;
                    editBtn.title = "Edit Widget Settings";
                    editBtn.style.cssText = "background: none; border: none; color: var(--color-primary); cursor: pointer; padding: 4px; line-height: 0; display: inline-flex; align-items: center; justify-content: center; border-radius: 4px; transition: color 0.2s, background 0.2s;";
                    editBtn.addEventListener("click", (e) => {
                        e.stopPropagation();
                        e.preventDefault();
                        const col = w.dataset.originalCol;
                        const idx = parseInt(w.dataset.originalIdx);
                        const nestedIdx = w.dataset.originalNestedIdx !== undefined ? parseInt(w.dataset.originalNestedIdx) : undefined;
                        openEditWidgetModal(col, idx, nestedIdx);
                    });

                    // Delete/trash button using inline SVG
                    const deleteBtn = document.createElement("button");
                    deleteBtn.type = "button";
                    deleteBtn.className = "widget-delete-btn";
                    deleteBtn.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/></svg>`;
                    deleteBtn.title = "Delete Widget";
                    deleteBtn.style.cssText = "background: none; border: none; color: var(--color-negative); cursor: pointer; padding: 4px; line-height: 0; display: inline-flex; align-items: center; justify-content: center; border-radius: 4px; transition: color 0.2s, background 0.2s;";
                    deleteBtn.addEventListener("click", async (e) => {
                        e.stopPropagation();
                        e.preventDefault();
                        const titleEl = w.querySelector(".uppercase");
                        const title = titleEl ? titleEl.innerText : "Widget";
                        if (await showConfirmModal(`Are you sure you want to delete the "${title}" widget?`)) {
                            if (w.dataset.originalNestedIdx !== undefined) {
                                await deleteWidget(`${w.dataset.originalCol}:${w.dataset.originalIdx}`, parseInt(w.dataset.originalNestedIdx));
                            } else {
                                await deleteWidget(w.dataset.originalCol, parseInt(w.dataset.originalIdx));
                            }
                        }
                    });

                    actionsDiv.appendChild(editBtn);
                    actionsDiv.appendChild(deleteBtn);
                    header.appendChild(actionsDiv);
                }
            }
        } else {
            w.classList.remove("editable");
            const actionsDiv = w.querySelector(".widget-edit-actions");
            if (actionsDiv) actionsDiv.remove();
            const handle = w.querySelector(".widget-drag-handle");
            if (handle) handle.remove();
        }
    };

    allCols.forEach(colObj => {
        const col = colObj.el;
        const colIdx = colObj.index;
        col.dataset.colIndex = colIdx;
        
        // Select only direct child widgets of this column
        const colWidgets = col.querySelectorAll(":scope > .widget");
        colWidgets.forEach((w, wIdx) => {
            // Check if it is a Group widget
            if (w.classList.contains("widget-type-group")) {
                const nestedGroup = w.querySelector(".widget-group");
                if (nestedGroup) {
                    nestedGroup.dataset.colIndex = `${colIdx}:${wIdx}`;
                    const nestedWidgets = nestedGroup.querySelectorAll(":scope > .widget");
                    nestedWidgets.forEach((nw, nwIdx) => {
                        setupWidget(nw, colIdx, wIdx, nwIdx);
                    });
                }
            }
            setupWidget(w, colIdx, wIdx);
        });
    });

    if (!enabled) return;
}

function getDragAfterElement(container, y) {
    // Select only direct child widgets to avoid offset calculations from nested widgets
    // Exclude the active drag placeholder to prevent cursor jitter
    const draggableElements = [...container.querySelectorAll(":scope > .widget:not(.dragging):not(.widget-placeholder)")];

    return draggableElements.reduce((closest, child) => {
        const box = child.getBoundingClientRect();
        const offset = y - box.top - box.height / 2;
        if (offset < 0 && offset > closest.offset) {
            return { offset: offset, element: child };
        } else {
            return closest;
        }
    }, { offset: Number.NEGATIVE_INFINITY }).element;
}

// Serialize new DOM positions and save back to backend YAML config
async function saveLayout() {
    const btnSave = document.getElementById("btn-save-layout");
    const btnCancel = document.getElementById("btn-cancel-layout");
    const header = document.querySelector(".header-container");
    
    if (btnSave) {
        btnSave.disabled = true;
        btnSave.textContent = "Saving...";
        btnSave.style.opacity = "0.7";
    }
    if (btnCancel) btnCancel.disabled = true;
    if (header) header.style.pointerEvents = "none";

    const headCol = document.querySelector(".page-column-head");
    const columns = document.querySelectorAll(".page-column");
    
    // Helper function to serialize a single widget (recursive for Group widgets)
    function serializeWidget(w) {
        if (w.classList.contains("widget-type-group")) {
            const nestedGroup = w.querySelector(".widget-group");
            const children = [];
            if (nestedGroup) {
                const nestedWidgets = nestedGroup.querySelectorAll(":scope > .widget");
                nestedWidgets.forEach(nw => {
                    children.push(serializeWidget(nw));
                });
            }
            const baseId = `${w.dataset.originalCol}:${w.dataset.originalIdx}`;
            return `${baseId}[${children.join(",")}]`;
        }
        
        if (w.dataset.originalNestedIdx !== undefined) {
            return `${w.dataset.originalCol}:${w.dataset.originalIdx}:${w.dataset.originalNestedIdx}`;
        }
        return `${w.dataset.originalCol}:${w.dataset.originalIdx}`;
    }

    const payloadHead = [];
    if (headCol) {
        const headWidgets = headCol.querySelectorAll(":scope > .widget");
        headWidgets.forEach(w => {
            payloadHead.push(serializeWidget(w));
        });
    }

    const payloadColumns = [];
    const payloadColumnSizes = [];
    columns.forEach(col => {
        const colWidgets = col.querySelectorAll(":scope > .widget");
        const colWidgetsArr = [];
        colWidgets.forEach(w => {
            colWidgetsArr.push(serializeWidget(w));
        });
        payloadColumns.push(colWidgetsArr);
        
        if (col.classList.contains("page-column-full")) {
            payloadColumnSizes.push("full");
        } else if (col.classList.contains("page-column-small")) {
            payloadColumnSizes.push("small");
        }
    });

    try {
        const response = await fetch("/api/layout/save", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                page: pageData.slug,
                head: payloadHead,
                columns: payloadColumns,
                column_sizes: payloadColumnSizes
            })
        });

        if (response.ok) {
            toggleEditMode(false);
            showToast("Layout saved successfully", "success");
            await refreshPageContentsLive();
        } else {
            const err = await response.text();
            showToast("Failed to save layout: " + err, "error");
        }
    } catch (err) {
        showToast("Network error saving layout: " + err.message, "error");
    }

    if (btnSave) {
        btnSave.disabled = false;
        btnSave.textContent = "Save";
        btnSave.style.opacity = "";
    }
    if (btnCancel) btnCancel.disabled = false;
    if (header) header.style.pointerEvents = "";
}

// Send widget deletion command to backend
async function deleteWidget(colIdx, widgetIdx) {
    const response = await fetch("/api/widgets/delete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            page: pageData.slug,
            column: colIdx.toString(),
            widget: widgetIdx
        })
    });

    if (response.ok) {
        showToast("Widget deleted", "success");
        await refreshPageContentsLive();
    } else {
        const err = await response.text();
        showToast("Failed to delete widget: " + err, "error");
    }
}

// ----------------------------------------------------
// Widget Addition Modal & Form Generation
// ----------------------------------------------------

const widgetFieldTemplates = {
    weather: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Location</label>
        <input type="text" name="location" placeholder="e.g. London, United Kingdom" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Units</label>
        <select name="units" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
            <option value="metric">Celsius (Metric)</option>
            <option value="imperial">Fahrenheit (Imperial)</option>
        </select>
    `,
    iframe: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">IFrame URL</label>
        <input type="url" name="url" placeholder="https://example.com" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Height</label>
        <input type="number" name="height" value="300" min="50" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    reddit: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Subreddit</label>
        <input type="text" name="subreddit" placeholder="e.g. selfhosted" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    rss: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">RSS Feed URLs</label>
        <div class="rss-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-rss-feed" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit;">+ Add Feed URL</button>
    `,
    /* 
       Stocks widget template with dynamic stock items list 
       and sort/style settings.
    */
    stocks: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Stock Symbols & Names</label>
        <span style="display: block; font-size: 0.8em; opacity: 0.6; margin-bottom: 8px;">Data provided by Yahoo Finance</span>
        <div class="stocks-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-stock-symbol" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 12px;">+ Add Symbol</button>
        <div style="display: flex; gap: 15px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Sort By</label>
                <select name="sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default (Order defined)</option>
                    <option value="absolute-change">Absolute Change</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Style</label>
                <select name="style" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default List</option>
                    <option value="dynamic-columns-experimental">Dynamic Columns (Experimental)</option>
                    <option value="horizontal-cards">Horizontal Cards</option>
                </select>
            </div>
        </div>
    `,
    /* 
       Markets widget template with dynamic market items list 
       and sort/style settings.
    */
    markets: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Market Symbols & Names</label>
        <span style="display: block; font-size: 0.8em; opacity: 0.6; margin-bottom: 8px;">Data provided by Yahoo Finance</span>
        <div class="stocks-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-stock-symbol" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 12px;">+ Add Symbol</button>
        <div style="display: flex; gap: 15px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Sort By</label>
                <select name="sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default (Order defined)</option>
                    <option value="absolute-change">Absolute Change</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Style</label>
                <select name="style" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default List</option>
                    <option value="dynamic-columns-experimental">Dynamic Columns (Experimental)</option>
                    <option value="horizontal-cards">Horizontal Cards</option>
                </select>
            </div>
        </div>
    `,
    videos: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">YouTube Channel IDs</label>
        <div class="videos-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-videos-channel" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit;">+ Add Channel ID</button>
    `,
    "twitch-channels": `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Twitch Channel Names</label>
        <div class="twitch-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-twitch-channel" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit;">+ Add Channel</button>
    `,
    repository: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">GitHub Repository (owner/repo)</label>
        <input type="text" name="repository" placeholder="e.g. glanceapp/glance" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">GitHub Token (Optional)</label>
        <input type="password" name="token" placeholder="e.g. ghp_..." style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <div style="display: flex; gap: 15px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Pull Requests Limit</label>
                <input type="number" name="pull-requests-limit" value="3" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Issues Limit</label>
                <input type="number" name="issues-limit" value="3" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
    `,
    releases: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">GitHub Repositories (owner/repo)</label>
        <div class="releases-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-release-repo" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 10px;">+ Add Repository</button>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">GitHub Token (Optional)</label>
        <input type="password" name="token" placeholder="e.g. ghp_..." style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Release Limit</label>
        <input type="number" name="limit" value="10" min="1" max="100" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Collapse After</label>
        <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    monitor: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Monitored Sites</label>
        <div class="monitor-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-monitor-site" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; transition: opacity 0.2s;">+ Add Another Site</button>
    `,
    bookmarks: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Group Title</label>
        <input type="text" name="group_title" placeholder="e.g. My Links" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 12px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Bookmark Links</label>
        <div class="bookmark-links" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-bookmark-link" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; transition: opacity 0.2s;">+ Add Another Link</button>
    `,
    clock: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Hour Format</label>
        <select name="hour-format" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
            <option value="24h">24 Hour (e.g. 17:00)</option>
            <option value="12h">12 Hour (e.g. 5:00 PM)</option>
        </select>
    `,
    "custom-api": `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">HTML/Go-Template Content</label>
        <textarea name="template" placeholder="<div>{{ .Title }}</div>" required style="width: 100%; height: 120px; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; resize: vertical; outline: none;"></textarea>
    `,
    "hacker-news": `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Sort By</label>
        <select name="sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;">
            <option value="top">Top</option>
            <option value="new">New</option>
            <option value="best">Best</option>
        </select>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Extra Sort By</label>
        <select name="extra-sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;">
            <option value="">None</option>
            <option value="engagement">Engagement</option>
        </select>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Post Limit</label>
        <input type="number" name="limit" value="15" min="1" max="100" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Collapse After</label>
        <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Comments URL Template (Optional)</label>
        <input type="text" name="comments-url-template" placeholder="https://news.ycombinator.com/item?id={POST-ID}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    spotify: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Spotify Client ID (Optional)</label>
        <input type="text" name="client_id" placeholder="Your Client ID" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Spotify Client Secret (Optional)</label>
        <input type="password" name="client_secret" placeholder="Your Client Secret" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Spotify Redirect URL (Optional)</label>
        <input type="url" name="redirect_url" placeholder="http://localhost:8086/api/spotify/callback" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Spotify Access Token (Optional)</label>
        <input type="password" name="access_token" placeholder="BQA..." style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Spotify Refresh Token (Optional)</label>
        <input type="password" name="refresh_token" placeholder="AQB..." style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    "twitch-top-games": `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Excluded Game Titles</label>
        <div class="twitch-exclude-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-twitch-exclude" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 10px;">+ Add Excluded Game</button>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Limit</label>
        <input type="number" name="limit" value="10" min="1" max="100" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Collapse After</label>
        <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    neuralwatt: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">NeuralWatt API Key</label>
        <input type="password" name="api-key" placeholder="sk-..." required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Update Interval (minutes)</label>
        <input type="number" name="update-interval" value="15" min="1" max="1440" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    "server-stats": `
        <p style="font-size: 0.85em; opacity: 0.7; margin-bottom: 10px;">Zeigt CPU, RAM, Disk, Docker und Uptime des Servers an.</p>
    `
};

// Modal visibility tracking for body class toggling and tab-trapping
function setModalOpen(isOpen) {
    if (isOpen) {
        document.body.classList.add("modal-open");
    } else {
        document.body.classList.remove("modal-open");
    }
}

function setupAddWidgetModal() {
    const modal = document.getElementById("add-widget-modal");
    const form = document.getElementById("add-widget-form");
    const typeSelect = document.getElementById("widget-type-select");
    const columnSelect = document.getElementById("widget-column-select");
    const fieldsContainer = document.getElementById("widget-fields-container");

    const showModal = () => {
        columnSelect.innerHTML = "";
        const columns = document.querySelectorAll(".page-column");
        columns.forEach((col, idx) => {
            const size = col.classList.contains("page-column-full") ? "Full" : "Small";
            columnSelect.appendChild(new Option(`Column ${idx + 1} (${size})`, idx));
        });

        typeSelect.dispatchEvent(new Event("change"));
        modal.style.display = "flex";
        document.body.style.overflow = "hidden";
        setModalOpen(true);
    };

    const hideModal = () => {
        modal.style.display = "none";
        form.reset();
        document.body.style.overflow = "";
        setModalOpen(false);
    };

    typeSelect.addEventListener("change", () => {
        const type = typeSelect.value;
        fieldsContainer.innerHTML = widgetFieldTemplates[type] || `<p style="font-size:0.85em; opacity:0.6; text-align:center;">This widget type does not require any properties.</p>`;

        const hideTitleWrapper = document.createElement("div");
        hideTitleWrapper.style.cssText = "margin-bottom: 15px; padding-bottom: 10px; border-bottom: 1px dashed var(--color-separator);";
        hideTitleWrapper.innerHTML = `
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="hide-title" style="cursor: pointer;" />
                Hide Widget Title
            </label>
        `;
        fieldsContainer.insertBefore(hideTitleWrapper, fieldsContainer.firstChild);

        // Wire dynamic add buttons and lists for dynamic fields
        initDynamicFields(fieldsContainer, type);
    });

    document.getElementById("btn-add-widget").addEventListener("click", showModal);
    const mobileAdd = document.getElementById("mobile-btn-add-widget");
    if (mobileAdd) {
        mobileAdd.addEventListener("click", (e) => {
            e.preventDefault();
            showModal();
        });
    }
    document.getElementById("btn-modal-cancel").addEventListener("click", hideModal);
    modal.addEventListener("click", (e) => {
        if (e.target === modal) hideModal();
    });

    form.addEventListener("submit", async (e) => {
        e.preventDefault();
        const formData = new FormData(form);
        const properties = {};

        formData.forEach((value, key) => {
            if (key === "hide-title") return;
            if (key === "feeds" || key === "symbols" || key === "channels" || key === "repositories") {
                // Obsolete comma-separated keys; skipped to avoid noise
            } else if (key === "height" || key === "update-interval" || key === "limit" || key === "collapse-after" || key === "pull-requests-limit" || key === "issues-limit") {
                properties[key] = parseInt(value, 10);
            } else if (key === "site_title" || key === "site_url" || key === "link_title" || key === "link_url" || key === "group_title") {
                // Handled separately below
            } else if (key === "rss_url" || key === "rss_title" || key === "stocks_symbol" || key === "stocks_name" || key === "videos_channel" || key === "twitch_channel" || key === "repo_name" || key === "release_repo_name" || key === "twitch_exclude") {
                // Handled separately below
            } else {
                properties[key] = value;
            }
        });

        const hideTitleInput = form.elements["hide-title"];
        if (hideTitleInput) {
            properties["hide-title"] = hideTitleInput.checked;
        }

        const type = typeSelect.value;

        // Dynamic lists collector for adding a new widget
        if (type === "rss") {
            const list = [];
            const urls = formData.getAll("rss_url");
            const titles = formData.getAll("rss_title");
            for (let i = 0; i < urls.length; i++) {
                if (urls[i].trim()) {
                    const obj = { url: urls[i].trim() };
                    if (titles[i] && titles[i].trim()) {
                        obj.title = titles[i].trim();
                    }
                    list.push(obj);
                }
            }
            properties["feeds"] = list;
        }
        if (type === "stocks" || type === "markets") {
            const list = [];
            const symbols = formData.getAll("stocks_symbol");
            const names = formData.getAll("stocks_name");
            for (let i = 0; i < symbols.length; i++) {
                if (symbols[i].trim()) {
                    list.push({
                        symbol: symbols[i].trim(),
                        name: names[i] ? names[i].trim() : ""
                    });
                }
            }
            if (type === "markets") {
                properties["markets"] = list;
            } else {
                properties["stocks"] = list;
            }
        }
        if (type === "videos") {
            properties["channels"] = formData.getAll("videos_channel").map(s => s.trim()).filter(Boolean);
        }
        if (type === "twitch-channels") {
            properties["channels"] = formData.getAll("twitch_channel").map(s => s.trim()).filter(Boolean);
        }
        if (type === "releases") {
            properties["repositories"] = formData.getAll("release_repo_name").map(s => s.trim()).filter(Boolean);
        }
        if (type === "twitch-top-games") {
            properties["exclude"] = formData.getAll("twitch_exclude").map(s => s.trim()).filter(Boolean);
        }

        // Special handling for monitor site items
        if (type === "monitor") {
            const sites = [];
            const siteTitles = formData.getAll("site_title");
            const siteUrls = formData.getAll("site_url");
            for (let i = 0; i < siteUrls.length; i++) {
                if (siteUrls[i].trim()) {
                    sites.push({
                        title: siteTitles[i] ? siteTitles[i].trim() : "",
                        url: siteUrls[i].trim()
                    });
                }
            }
            properties["sites"] = sites;
        }

        // Special handling for bookmarks
        if (type === "bookmarks") {
            const links = [];
            const linkTitles = formData.getAll("link_title");
            const linkUrls = formData.getAll("link_url");
            for (let i = 0; i < linkUrls.length; i++) {
                if (linkUrls[i].trim()) {
                    links.push({
                        title: linkTitles[i] ? linkTitles[i].trim() : "",
                        url: linkUrls[i].trim()
                    });
                }
            }
            properties["groups"] = [{
                title: formData.get("group_title") ? formData.get("group_title").trim() : "Links",
                links: links
            }];
        }

        const response = await fetch("/api/widgets/add", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                page: pageData.slug,
                column: columnSelect.value.toString(),
                type: typeSelect.value,
                properties: properties
            })
        });

        if (response.ok) {
            hideModal();
            showToast("Widget added successfully", "success");
            await refreshPageContentsLive();
        } else {
            const err = await response.text();
            showToast("Failed to add widget: " + err, "error");
        }
    });
}

function setupHeaderControls() {
    const editHandler = () => toggleEditMode(true);
    const cancelHandler = async () => {
        toggleEditMode(false);
        await refreshPageContentsLive();
    };
    const saveHandler = saveLayout;

    document.getElementById("btn-edit-layout").addEventListener("click", editHandler);
    document.getElementById("btn-cancel-layout").addEventListener("click", cancelHandler);
    document.getElementById("btn-save-layout").addEventListener("click", saveHandler);

    const btnUndo = document.getElementById("btn-undo-layout");
    if (btnUndo) {
        btnUndo.addEventListener("click", (e) => {
            e.preventDefault();
            undoLastLayoutChange();
        });
    }
    
    // Wire mobile edit controls
    const mobileEdit = document.getElementById("mobile-btn-edit-layout");
    if (mobileEdit) {
        mobileEdit.addEventListener("click", (e) => {
            e.preventDefault();
            editHandler();
        });
    }
    const mobileCancel = document.getElementById("mobile-btn-cancel-layout");
    if (mobileCancel) {
        mobileCancel.addEventListener("click", (e) => {
            e.preventDefault();
            cancelHandler();
        });
    }
    const mobileSave = document.getElementById("mobile-btn-save-layout");
    if (mobileSave) {
        mobileSave.addEventListener("click", (e) => {
            e.preventDefault();
            saveHandler();
        });
    }
    
    // Wire layout selector to dynamically change page columns
    const selectLayout = document.getElementById("select-page-layout");
    if (selectLayout) {
        selectLayout.addEventListener("change", () => {
            const selectedLayout = selectLayout.value;
            const newSizes = selectedLayout.split(",");
            
            // 1. Gather all current widgets in order
            const allWidgets = [];
            const columns = document.querySelectorAll(".page-column");
            columns.forEach(col => {
                const colWidgets = col.querySelectorAll(":scope > .widget");
                colWidgets.forEach(w => {
                    allWidgets.push(w);
                });
            });

            // 2. Clear page columns
            const pageColumnsContainer = document.querySelector(".page-columns");
            if (!pageColumnsContainer) return;
            pageColumnsContainer.innerHTML = "";

            // 3. Create new columns and distribute widgets
            newSizes.forEach((size, idx) => {
                const colDiv = document.createElement("div");
                colDiv.className = `page-column page-column-${size}`;
                colDiv.dataset.colIndex = idx;
                pageColumnsContainer.appendChild(colDiv);
            });

            // 4. Distribute widgets evenly across the new columns
            const targetCols = pageColumnsContainer.querySelectorAll(".page-column");
            if (targetCols.length > 0) {
                allWidgets.forEach((w, idx) => {
                    const colIdx = idx % targetCols.length;
                    targetCols[colIdx].appendChild(w);
                });
            }

            // Sync mobile navigation radio pills dynamically to match new columns count
            const mobileIconsContainer = document.querySelector(".mobile-navigation-icons");
            if (mobileIconsContainer) {
                // Remove existing radio labels
                const existingLabels = mobileIconsContainer.querySelectorAll("label:has(input[name='column'])");
                existingLabels.forEach(lbl => lbl.remove());

                const hamburgerLabel = mobileIconsContainer.querySelector("label:has(.mobile-navigation-page-links-input)");

                newSizes.forEach((size, idx) => {
                    const label = document.createElement("label");
                    label.className = "mobile-navigation-label";
                    
                    const radio = document.createElement("input");
                    radio.type = "radio";
                    radio.className = "mobile-navigation-input";
                    radio.name = "column";
                    radio.value = idx;
                    radio.autocomplete = "off";
                    if (idx === 0) {
                        radio.checked = true;
                    }

                    const pill = document.createElement("div");
                    pill.className = "mobile-navigation-pill";

                    label.appendChild(radio);
                    label.appendChild(pill);

                    if (hamburgerLabel) {
                        mobileIconsContainer.insertBefore(label, hamburgerLabel);
                    } else {
                        mobileIconsContainer.appendChild(label);
                    }
                });
            }

            // 5. Re-run draggability setup so that DND is configured on the new column elements
            enableWidgetsDraggability(true);
            pushLayoutHistory();
        });
    }

    // Add page click handler
    const btnAddPage = document.getElementById("btn-add-page");
    if (btnAddPage) {
        btnAddPage.addEventListener("click", async () => {
            const pageName = await showPromptModal("Enter a name for the new dashboard page:");
            if (!pageName || !pageName.trim()) return;

            try {
                const response = await fetch("/api/pages/add", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ name: pageName.trim() })
                });

                if (response.ok) {
                    window.location.reload();
                } else {
                    const err = await response.text();
                    showToast("Failed to add page: " + err, "error");
                }
            } catch (err) {
                showToast("Error: " + err.message, "error");
            }
        });
    }
}

// Sets up modal state management and form submission for server settings
function setupSettingsMenu() {
    const logoG = document.getElementById("logo-g");
    const modal = document.getElementById("settings-modal");
    const form = document.getElementById("settings-form");
    const cancelBtn = document.getElementById("btn-settings-cancel");

    if (!modal || !form) return;

    // Dynamically populate supported timezones
    const serverTimezoneSelect = form.elements["server_timezone"];
    if (serverTimezoneSelect) {
        while (serverTimezoneSelect.options.length > 1) {
            serverTimezoneSelect.remove(1);
        }
        try {
            const timezones = Intl.supportedValuesOf('timeZone');
            timezones.forEach(tz => {
                const opt = document.createElement("option");
                opt.value = tz;
                opt.textContent = tz;
                serverTimezoneSelect.appendChild(opt);
            });
        } catch (e) {
            console.error("Intl timezone listing not supported", e);
        }
    }

    // Fetch and display active configuration parameters
    const showSettings = async () => {
        try {
            const response = await fetch("/api/settings");
            if (!response.ok) throw new Error("Server returned status " + response.status);
            const data = await response.json();

            if (CLIENT_CONFIG.debugLogging) {
                console.log("[Settings] Loaded data:", data);
            }

            // Map Branding settings
            form.elements["branding_app_name"].value = (data.branding && data.branding["app-name"]) || "";
            form.elements["branding_custom_footer"].value = (data.branding && data.branding["custom-footer"]) || "";

            // Map server settings
            form.elements["server_host"].value = data.server.host || "";
            form.elements["server_port"].value = data.server.port || "";
            form.elements["server_assets_path"].value = data.server["assets-path"] || "";
            form.elements["server_timezone"].value = data.server.timezone || "";

            // Map Spotify credentials
            form.elements["spotify_client_id"].value = data.spotify["client-id"] || "";
            form.elements["spotify_client_secret"].value = data.spotify["client-secret"] || "";
            form.elements["spotify_redirect_url"].value = data.spotify["redirect-url"] || "";

            // Map Theme & style customizations
            form.elements["theme_light"].checked = data.theme.light || false;
            form.elements["theme_background_color"].value = data.theme["background-color"] || "";
            form.elements["theme_primary_color"].value = data.theme["primary-color"] || "";
            form.elements["theme_positive_color"].value = data.theme["positive-color"] || "";
            form.elements["theme_negative_color"].value = data.theme["negative-color"] || "";
            form.elements["theme_contrast_multiplier"].value = data.theme["contrast-multiplier"] !== undefined ? data.theme["contrast-multiplier"] : 1.0;
            form.elements["theme_text_saturation_multiplier"].value = data.theme["text-saturation-multiplier"] !== undefined ? data.theme["text-saturation-multiplier"] : 1.0;
            form.elements["theme_custom_css_file"].value = data.theme["custom-css-file"] || "";
            form.elements["theme_widget_gap"].value = (data.theme && data.theme["widget-gap"]) || "";
            form.elements["theme_widget_vertical_padding"].value = (data.theme && data.theme["widget-content-vertical-padding"]) || "";
            form.elements["theme_widget_horizontal_padding"].value = (data.theme && data.theme["widget-content-horizontal-padding"]) || "";
            form.elements["theme_border_radius"].value = (data.theme && data.theme["border-radius"]) || "";

        modal.style.display = "flex";
        document.body.style.overflow = "hidden";
        setModalOpen(true);
        } catch (err) {
            showToast("Failed to load settings: " + err.message, "error");
        }
    };

    const hideSettings = () => {
        modal.style.display = "none";
        form.reset();
        document.body.style.overflow = "";
        setModalOpen(false);
    };

    if (logoG) {
        logoG.addEventListener("click", showSettings);
    }

    const mobileSettings = document.getElementById("mobile-btn-settings");
    if (mobileSettings) {
        mobileSettings.addEventListener("click", (e) => {
            e.preventDefault();
            showSettings();
        });
    }

    if (cancelBtn) {
        cancelBtn.addEventListener("click", hideSettings);
    }

    modal.addEventListener("click", (e) => {
        if (e.target === modal) hideSettings();
    });

    form.addEventListener("submit", async (e) => {
        e.preventDefault();

        const newPort = parseInt(form.elements["server_port"].value, 10);
        const newHost = form.elements["server_host"].value || "127.0.0.1";
        
        // Assemble target payload for save API
        const payload = {
            branding: {
                "app-name": form.elements["branding_app_name"].value,
                "custom-footer": form.elements["branding_custom_footer"].value
            },
            server: {
                host: newHost,
                port: newPort,
                "assets-path": form.elements["server_assets_path"].value,
                timezone: form.elements["server_timezone"].value
            },
            theme: {
                light: form.elements["theme_light"].checked,
                "background-color": form.elements["theme_background_color"].value,
                "primary-color": form.elements["theme_primary_color"].value,
                "positive-color": form.elements["theme_positive_color"].value,
                "negative-color": form.elements["theme_negative_color"].value,
                "contrast-multiplier": parseFloat(form.elements["theme_contrast_multiplier"].value),
                "text-saturation-multiplier": parseFloat(form.elements["theme_text_saturation_multiplier"].value),
                "custom-css-file": form.elements["theme_custom_css_file"].value,
                "widget-gap": form.elements["theme_widget_gap"].value,
                "widget-content-vertical-padding": form.elements["theme_widget_vertical_padding"].value,
                "widget-content-horizontal-padding": form.elements["theme_widget_horizontal_padding"].value,
                "border-radius": form.elements["theme_border_radius"].value
            },
            spotify: {
                "client-id": form.elements["spotify_client_id"].value,
                "client-secret": form.elements["spotify_client_secret"].value,
                "redirect-url": form.elements["spotify_redirect_url"].value
            }
        };

        try {
            const response = await fetch("/api/settings/save", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payload)
            });

            if (response.ok) {
                // Instantly update the branding options live in the DOM
                const newAppName = form.elements["branding_app_name"].value || "Glance";
                const newFooterHTML = form.elements["branding_custom_footer"].value;

                // Update Logo letter (first letter of app name)
                const logoEl = document.getElementById("logo-g");
                if (logoEl) {
                    logoEl.textContent = newAppName.charAt(0);
                }

                // Update tab title
                const pageTitle = pageData.slug ? pageData.slug.charAt(0).toUpperCase() + pageData.slug.slice(1) : "Home";
                document.title = `${pageTitle} - ${newAppName}`;

                // Update Custom Footer
                const footerEl = document.querySelector(".footer");
                if (footerEl) {
                    if (newFooterHTML) {
                        footerEl.innerHTML = newFooterHTML;
                    } else {
                        footerEl.innerHTML = `<div><span class="size-h3">${newAppName}</span></div>`;
                    }
                }

                const currentPort = parseInt(window.location.port || (window.location.protocol === "https:" ? "443" : "80"), 10);
                if (newPort !== currentPort) {
                    alert(`Settings saved successfully. Port changed from ${currentPort} to ${newPort}. The application is reloading on the new port.`);
                    setTimeout(() => {
                        const hostPart = (newHost === "0.0.0.0" || newHost === "") ? window.location.hostname : newHost;
                        window.location.href = `${window.location.protocol}//${hostPart}:${newPort}`;
                    }, CLIENT_CONFIG.portRedirectDelayMs);
                } else {
                    window.location.reload();
                }
            } else {
                const err = await response.text();
                showToast("Failed to save settings: " + err, "error");
            }
        } catch (err) {
            showToast("Error sending request: " + err.message, "error");
        }
    });

    // Config import logic
    const importBtn = document.getElementById("btn-config-import");
    const importFile = document.getElementById("config-import-file");
    const importStatus = document.getElementById("config-import-status");

    if (importBtn && importFile) {
        importBtn.addEventListener("click", () => importFile.click());

        importFile.addEventListener("change", async () => {
            const file = importFile.files[0];
            if (!file) return;

            if (!file.name.endsWith(".yml") && !file.name.endsWith(".yaml")) {
                showToast("Only .yml or .yaml files are accepted.", "error");
                importFile.value = "";
                return;
            }

            if (!await showConfirmModal("Importing a config file will replace your current configuration entirely. Continue?")) {
                importFile.value = "";
                return;
            }

            importStatus.textContent = "Importing...";
            importBtn.disabled = true;

            try {
                const formData = new FormData();
                formData.append("file", file);

                const response = await fetch("/api/config/import", {
                    method: "POST",
                    body: formData
                });

                if (response.ok) {
                    showToast("Config imported successfully. Reloading...", "success");
                    setTimeout(() => window.location.reload(), 1500);
                } else {
                    const errText = await response.text();
                    showToast("Import failed: " + errText, "error");
                    importStatus.textContent = "";
                }
            } catch (err) {
                showToast("Error importing config: " + err.message, "error");
                importStatus.textContent = "";
            }

            importBtn.disabled = false;
            importFile.value = "";
        });
    }
}

// Updates all digital clock widgets with local time and date client-side
function setupClocks() {
    const updateClocks = () => {
        const clocks = document.querySelectorAll(".clock-widget");
        if (clocks.length === 0) return;

        const now = new Date();
        clocks.forEach(clock => {
            const format = clock.dataset.hourFormat || "24h";
            
            let hours = now.getHours();
            const minutes = String(now.getMinutes()).padStart(2, "0");
            let ampm = "";
            
            if (format === "12h") {
                ampm = hours >= 12 ? " PM" : " AM";
                hours = hours % 12;
                hours = hours ? hours : 12;
            }
            const timeStr = `${String(hours).padStart(2, "0")}:${minutes}${ampm}`;
            
            const options = { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' };
            const dateStr = now.toLocaleDateString('en-US', options);

            const timeEl = clock.querySelector(".clock-time");
            const dateEl = clock.querySelector(".clock-date");
            if (timeEl) timeEl.textContent = timeStr;
            if (dateEl) dateEl.textContent = dateStr;
        });
    };

    updateClocks();
    if (!window.clockIntervalInitialized) {
        window.clockIntervalInitialized = true;
        setInterval(updateClocks, 1000);
    }
}

// Re-fetches the page contents dynamically from the server and updates the DOM, re-binding all scripts and listeners.
async function refreshPageContentsLive() {
    const pageElement = document.getElementById("page");
    
    // Capture old stats values to animate updates
    const oldValues = {};
    if (pageElement) {
        pageElement.querySelectorAll('.nw-stat-value, .nw-today-value, .nw-gauge-value, .nw-hero-value').forEach((el, index) => {
            oldValues[index] = el.textContent.trim();
        });
    }

    const wasEditMode = document.body.classList.contains("layout-edit-mode");
    ignoreReloadPageUntil = Date.now() + RELOAD_PAGE_IGNORE_DURATION_MS;
    
    try {
        const pageContents = await fetchPageContents(pageData.slug);
        pageElement.innerHTML = pageContents;
        
        // Match elements and highlight any changes with a flash transition
        pageElement.querySelectorAll('.nw-stat-value, .nw-today-value, .nw-gauge-value, .nw-hero-value').forEach((el, index) => {
            const oldValue = oldValues[index];
            const newValue = el.textContent.trim();
            if (oldValue !== undefined && oldValue !== newValue) {
                el.classList.add('nw-value-updated');
                setTimeout(() => el.classList.remove('nw-value-updated'), 1000);
            }
        });
    } catch (e) {
        console.error("[Refresh] Failed to reload page contents:", e);
        return;
    }

    // Sync mobile navigation radio pills dynamically to match current columns count in DOM
    const columns = pageElement.querySelectorAll(".page-column");
    const mobileIconsContainer = document.querySelector(".mobile-navigation-icons");
    if (mobileIconsContainer && columns.length > 0) {
        // Remove existing radio labels
        const existingLabels = mobileIconsContainer.querySelectorAll("label:has(input[name='column'])");
        existingLabels.forEach(lbl => lbl.remove());

        const hamburgerLabel = mobileIconsContainer.querySelector("label:has(.mobile-navigation-page-links-input)");

        columns.forEach((col, idx) => {
            const label = document.createElement("label");
            label.className = "mobile-navigation-label";
            
            const radio = document.createElement("input");
            radio.type = "radio";
            radio.className = "mobile-navigation-input";
            radio.name = "column";
            radio.value = idx;
            radio.autocomplete = "off";
            if (idx === 0) {
                radio.checked = true;
            }

            const pill = document.createElement("div");
            pill.className = "mobile-navigation-pill";

            label.appendChild(radio);
            label.appendChild(pill);

            if (hamburgerLabel) {
                mobileIconsContainer.insertBefore(label, hamburgerLabel);
            } else {
                mobileIconsContainer.appendChild(label);
            }
        });
    }
    
    setupLazyImages();
    setupCarousels();
    setupClocks();
    setupDynamicRelativeTime();

    // Apply cached Spotify auth state before WS reconnects to prevent UI flash
    const cachedAuth = localStorage.getItem("spotify_last_auth");
    if (cachedAuth === "true") {
        updateSpotifyWidget({ authorized: true, track: null });
    }
    if (lastSpotifyState) {
        updateSpotifyWidget(lastSpotifyState);
    }
    
    if (wasEditMode) {
        enableWidgetsDraggability(true);
    }
}

// ----------------------------------------------------
// Dynamic Fields Helper Functions
// ----------------------------------------------------

/**
 * Renders a simple single text/url input field with a delete button.
 * Used for arrays of strings (e.g. YouTube Channel IDs, RSS feed URLs).
 */
function addSingleStringInput(container, inputName, placeholder, value, inputType = "text") {
    const div = document.createElement("div");
    div.style.cssText = "display: flex; gap: 8px; margin-bottom: 8px; align-items: center;";
    div.innerHTML = `
        <input type="${inputType}" name="${inputName}" placeholder="${placeholder}" required value="${value}" style="flex: 1; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
        <button type="button" class="btn-remove-dynamic-item" style="background: transparent; border: 1px solid var(--color-negative); color: var(--color-negative); border-radius: 4px; padding: 0 10px; height: 34px; font-size: 1.2rem; cursor: pointer; transition: all 0.2s ease; font-weight: bold;">×</button>
    `;
    container.appendChild(div);
    const btn = div.querySelector(".btn-remove-dynamic-item");
    btn.addEventListener("click", () => div.remove());
    btn.addEventListener("mouseover", () => {
        btn.style.backgroundColor = "var(--color-negative)";
        btn.style.color = "#ffffff";
    });
    btn.addEventListener("mouseout", () => {
        btn.style.backgroundColor = "transparent";
        btn.style.color = "var(--color-negative)";
    });
}

/**
 * Renders a stock symbol input next to a custom display name input.
 */
function addStockInputField(container, symbol, name) {
    const div = document.createElement("div");
    div.className = "stocks-item";
    div.style.cssText = "display: flex; gap: 8px; margin-bottom: 8px; align-items: center;";
    div.innerHTML = `
        <input type="text" name="stocks_symbol" placeholder="Symbol (e.g. AAPL)" required value="${symbol}" style="width: 120px; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
        <input type="text" name="stocks_name" placeholder="Name (e.g. Apple)" value="${name}" style="flex: 1; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
        <button type="button" class="btn-remove-dynamic-item" style="background: transparent; border: 1px solid var(--color-negative); color: var(--color-negative); border-radius: 4px; padding: 0 10px; height: 34px; font-size: 1.2rem; cursor: pointer; transition: all 0.2s ease; font-weight: bold;">×</button>
    `;
    container.appendChild(div);
    const btn = div.querySelector(".btn-remove-dynamic-item");
    btn.addEventListener("click", () => div.remove());
    btn.addEventListener("mouseover", () => {
        btn.style.backgroundColor = "var(--color-negative)";
        btn.style.color = "#ffffff";
    });
    btn.addEventListener("mouseout", () => {
        btn.style.backgroundColor = "transparent";
        btn.style.color = "var(--color-negative)";
    });
}

/**
 * Renders a site title and URL input block for the monitor widget.
 */
function addMonitorSiteInput(container, title, url) {
    const div = document.createElement("div");
    div.className = "monitor-item";
    div.style.cssText = "border-top: 1px dashed var(--color-widget-content-border); padding-top: 10px; margin-top: 10px; position: relative;";
    div.innerHTML = `
        <button type="button" class="btn-remove-site" style="position: absolute; right: 0; top: 10px; background: transparent; border: 1px solid var(--color-negative); color: var(--color-negative); border-radius: 4px; padding: 4px 8px; font-size: 0.8rem; font-family: inherit; font-weight: 600; cursor: pointer; transition: all 0.2s ease; z-index: 10; display: inline-flex; align-items: center; justify-content: center; line-height: 1;">× Remove</button>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Site Title</label>
        <input type="text" name="site_title" placeholder="e.g. Google" required value="${title}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 8px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Site URL</label>
        <input type="url" name="site_url" placeholder="https://google.com" required value="${url}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `;
    container.appendChild(div);
    const btn = div.querySelector(".btn-remove-site");
    btn.addEventListener("click", () => div.remove());
    btn.addEventListener("mouseover", () => {
        btn.style.backgroundColor = "var(--color-negative)";
        btn.style.color = "#ffffff";
    });
    btn.addEventListener("mouseout", () => {
        btn.style.backgroundColor = "transparent";
        btn.style.color = "var(--color-negative)";
    });
}

/**
 * Renders a link title and URL input block for the bookmarks widget.
 */
function addBookmarkLinkInput(container, title, url) {
    const div = document.createElement("div");
    div.className = "bookmark-link-item";
    div.style.cssText = "border-top: 1px dashed var(--color-widget-content-border); padding-top: 10px; margin-top: 10px; position: relative;";
    div.innerHTML = `
        <button type="button" class="btn-remove-link" style="position: absolute; right: 0; top: 10px; background: transparent; border: 1px solid var(--color-negative); color: var(--color-negative); border-radius: 4px; padding: 4px 8px; font-size: 0.8rem; font-family: inherit; font-weight: 600; cursor: pointer; transition: all 0.2s ease; z-index: 10; display: inline-flex; align-items: center; justify-content: center; line-height: 1;">× Remove</button>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Link Title</label>
        <input type="text" name="link_title" placeholder="e.g. My Link" required value="${title}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 8px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Link URL</label>
        <input type="url" name="link_url" placeholder="https://example.com" required value="${url}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `;
    container.appendChild(div);
    const btn = div.querySelector(".btn-remove-link");
    btn.addEventListener("click", () => div.remove());
    btn.addEventListener("mouseover", () => {
        btn.style.backgroundColor = "var(--color-negative)";
        btn.style.color = "#ffffff";
    });
    btn.addEventListener("mouseout", () => {
        btn.style.backgroundColor = "transparent";
        btn.style.color = "var(--color-negative)";
    });
}

/**
 * Initializes list field values dynamically in the edit or add form.
 * Wires the "Add Item" button listener.
 */
function initDynamicFields(container, type, widget) {
    if (type === "rss") {
        const itemsDiv = container.querySelector(".rss-items");
        const btnAdd = container.querySelector("#btn-add-rss-feed");
        if (!itemsDiv || !btnAdd) return;

        let feeds = [];
        if (widget && widget.feeds) {
            feeds = widget.feeds.map(f => (typeof f === "object" && f !== null) ? { url: f.url || "", title: f.title || "" } : { url: f || "", title: "" });
        }

        itemsDiv.innerHTML = "";
        feeds.forEach(val => addRSSFeedInput(itemsDiv, val.url, val.title));
        if (feeds.length === 0) addRSSFeedInput(itemsDiv, "", "");

        btnAdd.addEventListener("click", () => addRSSFeedInput(itemsDiv, "", ""));
    }
    else if (type === "stocks" || type === "markets") {
        const itemsDiv = container.querySelector(".stocks-items");
        const btnAdd = container.querySelector("#btn-add-stock-symbol");
        if (!itemsDiv || !btnAdd) return;

        const stockList = (widget && (widget.markets || widget.stocks)) || [];
        itemsDiv.innerHTML = "";
        stockList.forEach(stock => {
            const sym = typeof stock === "object" ? (stock.symbol || "") : stock;
            const name = typeof stock === "object" ? (stock.name || "") : "";
            addStockInputField(itemsDiv, sym, name);
        });
        if (stockList.length === 0) addStockInputField(itemsDiv, "", "");

        btnAdd.addEventListener("click", () => addStockInputField(itemsDiv, "", ""));
    }
    else if (type === "videos") {
        const itemsDiv = container.querySelector(".videos-items");
        const btnAdd = container.querySelector("#btn-add-videos-channel");
        if (!itemsDiv || !btnAdd) return;

        const channels = (widget && widget.channels) || [];
        itemsDiv.innerHTML = "";
        channels.forEach(val => addSingleStringInput(itemsDiv, "videos_channel", "e.g. UCsBjURrPoezykLs9EqgamOA", val));
        if (channels.length === 0) addSingleStringInput(itemsDiv, "videos_channel", "e.g. UCsBjURrPoezykLs9EqgamOA", "");

        btnAdd.addEventListener("click", () => addSingleStringInput(itemsDiv, "videos_channel", "e.g. UCsBjURrPoezykLs9EqgamOA", ""));
    }
    else if (type === "twitch-channels") {
        const itemsDiv = container.querySelector(".twitch-items");
        const btnAdd = container.querySelector("#btn-add-twitch-channel");
        if (!itemsDiv || !btnAdd) return;

        const channels = (widget && widget.channels) || [];
        itemsDiv.innerHTML = "";
        channels.forEach(val => addSingleStringInput(itemsDiv, "twitch_channel", "e.g. xqc", val));
        if (channels.length === 0) addSingleStringInput(itemsDiv, "twitch_channel", "e.g. xqc", "");

        btnAdd.addEventListener("click", () => addSingleStringInput(itemsDiv, "twitch_channel", "e.g. xqc", ""));
    }
    else if (type === "releases") {
        const itemsDiv = container.querySelector(".releases-items");
        const btnAdd = container.querySelector("#btn-add-release-repo");
        if (!itemsDiv || !btnAdd) return;

        const repos = (widget && widget.repositories) || [];
        itemsDiv.innerHTML = "";
        repos.forEach(val => addSingleStringInput(itemsDiv, "release_repo_name", "e.g. glanceapp/glance", val));
        if (repos.length === 0) addSingleStringInput(itemsDiv, "release_repo_name", "e.g. glanceapp/glance", "");

        btnAdd.addEventListener("click", () => addSingleStringInput(itemsDiv, "release_repo_name", "e.g. glanceapp/glance", ""));
    }
    else if (type === "twitch-top-games") {
        const itemsDiv = container.querySelector(".twitch-exclude-items");
        const btnAdd = container.querySelector("#btn-add-twitch-exclude");
        if (!itemsDiv || !btnAdd) return;

        const exclude = (widget && widget.exclude) || [];
        itemsDiv.innerHTML = "";
        exclude.forEach(val => addSingleStringInput(itemsDiv, "twitch_exclude", "e.g. Just Chatting", val));
        if (exclude.length === 0) addSingleStringInput(itemsDiv, "twitch_exclude", "e.g. Just Chatting", "");

        btnAdd.addEventListener("click", () => addSingleStringInput(itemsDiv, "twitch_exclude", "e.g. Just Chatting", ""));
    }
    else if (type === "monitor") {
        const itemsDiv = container.querySelector(".monitor-items");
        const btnAdd = container.querySelector("#btn-add-monitor-site");
        if (!itemsDiv || !btnAdd) return;

        const sites = (widget && widget.sites) || [];
        itemsDiv.innerHTML = "";
        sites.forEach(site => addMonitorSiteInput(itemsDiv, site.title || "", site.url || ""));
        if (sites.length === 0) addMonitorSiteInput(itemsDiv, "", "");

        btnAdd.addEventListener("click", () => addMonitorSiteInput(itemsDiv, "", ""));
    }
    else if (type === "bookmarks") {
        const itemsDiv = container.querySelector(".bookmark-links");
        const btnAdd = container.querySelector("#btn-add-bookmark-link");
        if (!itemsDiv || !btnAdd) return;

        const groups = (widget && widget.groups) || [];
        const links = (groups.length > 0 && groups[0].links) || [];
        itemsDiv.innerHTML = "";
        links.forEach(link => addBookmarkLinkInput(itemsDiv, link.title || "", link.url || ""));
        if (links.length === 0) addBookmarkLinkInput(itemsDiv, "", "");

        btnAdd.addEventListener("click", () => addBookmarkLinkInput(itemsDiv, "", ""));
    }
}

/**
 * Appends a single RSS URL and Title input.
 */
function addRSSFeedInput(container, url, title) {
    const div = document.createElement("div");
    div.className = "rss-item";
    div.style.cssText = "display: flex; gap: 8px; margin-bottom: 8px; align-items: center;";
    div.innerHTML = `
        <input type="url" name="rss_url" placeholder="Feed URL (e.g. https://selfh.st/rss/)" required value="${url}" style="flex: 1; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
        <input type="text" name="rss_title" placeholder="Title Override" value="${title}" style="width: 130px; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
        <button type="button" class="btn-remove-dynamic-item" style="background: transparent; border: 1px solid var(--color-negative); color: var(--color-negative); border-radius: 4px; padding: 0 10px; height: 34px; font-size: 1.2rem; cursor: pointer; transition: all 0.2s ease; font-weight: bold;">×</button>
    `;
    container.appendChild(div);
    const btn = div.querySelector(".btn-remove-dynamic-item");
    btn.addEventListener("click", () => div.remove());
    btn.addEventListener("mouseover", () => {
        btn.style.backgroundColor = "var(--color-negative)";
        btn.style.color = "#ffffff";
    });
    btn.addEventListener("mouseout", () => {
        btn.style.backgroundColor = "transparent";
        btn.style.color = "var(--color-negative)";
    });
}

// ----------------------------------------------------
// Edit Widget Modal & Config Prefills
// ----------------------------------------------------

async function openEditWidgetModal(col, idx, nestedIdx) {
    const modal = document.getElementById("edit-widget-modal");
    const form = document.getElementById("edit-widget-form");
    const fieldsContainer = document.getElementById("edit-widget-fields-container");
    const titleHeader = document.getElementById("edit-widget-title");

    const nestedParam = nestedIdx !== undefined ? `&widget=${nestedIdx}` : `&widget=${idx}`;
    const colParam = nestedIdx !== undefined ? `&column=${col}:${idx}` : `&column=${col}`;
    const url = `/api/widgets/get?page=${pageData.slug}${colParam}${nestedParam}`;
    
    try {
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(await response.text());
        }
        const widget = await response.json();
        const type = widget.type;

        titleHeader.innerText = `Edit ${type.charAt(0).toUpperCase() + type.slice(1)} Widget`;
        
        document.getElementById("edit-widget-col").value = col;
        document.getElementById("edit-widget-idx").value = idx;
        document.getElementById("edit-widget-nested-idx").value = nestedIdx !== undefined ? nestedIdx : "";
        document.getElementById("edit-widget-type").value = type;

        fieldsContainer.innerHTML = widgetFieldTemplates[type] || `<p style="font-size:0.85em; opacity:0.6; text-align:center;">This widget type does not require any properties.</p>`;

        const hideTitleWrapper = document.createElement("div");
        hideTitleWrapper.style.cssText = "margin-bottom: 15px; padding-bottom: 10px; border-bottom: 1px dashed var(--color-separator);";
        hideTitleWrapper.innerHTML = `
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="hide-title" style="cursor: pointer;" />
                Hide Widget Title
            </label>
        `;
        fieldsContainer.insertBefore(hideTitleWrapper, fieldsContainer.firstChild);

        prefillWidgetFields(fieldsContainer, type, widget);
        initDynamicFields(fieldsContainer, type, widget);

        const hideTitleCb = fieldsContainer.querySelector('[name="hide-title"]');
        if (hideTitleCb) {
            const ht = widget["hide-title"];
            hideTitleCb.checked = ht === true || ht === "true" || ht === "on" || ht === 1;
        }

        modal.style.display = "flex";
        document.body.style.overflow = "hidden";
    } catch (err) {
        showToast("Failed to fetch widget details: " + err.message, "error");
    }
}

function prefillWidgetFields(container, type, widget) {
    for (const key in widget) {
        if (key === "type" || key === "sites" || key === "groups" || key === "feeds" || key === "symbols" || key === "markets" || key === "stocks" || key === "channels" || key === "repositories" || key === "exclude") continue;
        const val = widget[key];

        const inputs = container.querySelectorAll(`[name="${key}"]`);
        inputs.forEach(input => {
            if (typeof val === "object" && val !== null) {
                // Skiped or custom handled
            } else {
                input.value = val;
            }
        });
    }
}

function setupEditWidgetModal() {
    const modal = document.getElementById("edit-widget-modal");
    const form = document.getElementById("edit-widget-form");

    const hideModal = () => {
        modal.style.display = "none";
        form.reset();
        document.body.style.overflow = "";
        setModalOpen(false);
    };

    document.getElementById("btn-edit-modal-cancel").addEventListener("click", hideModal);
    modal.addEventListener("click", (e) => {
        if (e.target === modal) hideModal();
    });

    form.addEventListener("submit", async (e) => {
        e.preventDefault();

        const col = document.getElementById("edit-widget-col").value;
        const idx = document.getElementById("edit-widget-idx").value;
        const nestedIdxVal = document.getElementById("edit-widget-nested-idx").value;
        const type = document.getElementById("edit-widget-type").value;

        const formData = new FormData(form);
        const properties = {};

        formData.forEach((value, key) => {
            if (key === "hide-title") return;
            if (key === "feeds" || key === "symbols" || key === "channels" || key === "repositories" || key === "exclude") {
                // Obsolete comma-separated keys; skipped to avoid noise
            } else if (key === "height" || key === "limit" || key === "collapse-after" || key === "update-interval" || key === "pull-requests-limit" || key === "issues-limit") {
                properties[key] = parseInt(value, 10);
            } else if (key === "site_title" || key === "site_url" || key === "link_title" || key === "link_url" || key === "group_title") {
                // Handled separately below
            } else if (key === "rss_url" || key === "rss_title" || key === "stocks_symbol" || key === "stocks_name" || key === "videos_channel" || key === "twitch_channel" || key === "repo_name" || key === "release_repo_name" || key === "twitch_exclude") {
                // Handled separately below
            } else {
                properties[key] = value;
            }
        });

        const hideTitleInput = form.elements["hide-title"];
        if (hideTitleInput) {
            properties["hide-title"] = hideTitleInput.checked;
        }

        // Dynamic lists collector for editing a widget
        if (type === "rss") {
            const list = [];
            const urls = formData.getAll("rss_url");
            const titles = formData.getAll("rss_title");
            for (let i = 0; i < urls.length; i++) {
                if (urls[i].trim()) {
                    const obj = { url: urls[i].trim() };
                    if (titles[i] && titles[i].trim()) {
                        obj.title = titles[i].trim();
                    }
                    list.push(obj);
                }
            }
            properties["feeds"] = list;
        }
        if (type === "stocks" || type === "markets") {
            const list = [];
            const symbols = formData.getAll("stocks_symbol");
            const names = formData.getAll("stocks_name");
            for (let i = 0; i < symbols.length; i++) {
                if (symbols[i].trim()) {
                    list.push({
                        symbol: symbols[i].trim(),
                        name: names[i] ? names[i].trim() : ""
                    });
                }
            }
            if (type === "markets") {
                properties["markets"] = list;
            } else {
                properties["stocks"] = list;
            }
        }
        if (type === "videos") {
            properties["channels"] = formData.getAll("videos_channel").map(s => s.trim()).filter(Boolean);
        }
        if (type === "twitch-channels") {
            properties["channels"] = formData.getAll("twitch_channel").map(s => s.trim()).filter(Boolean);
        }
        if (type === "releases") {
            properties["repositories"] = formData.getAll("release_repo_name").map(s => s.trim()).filter(Boolean);
        }
        if (type === "twitch-top-games") {
            properties["exclude"] = formData.getAll("twitch_exclude").map(s => s.trim()).filter(Boolean);
        }

        if (type === "monitor") {
            const sites = [];
            const siteTitles = formData.getAll("site_title");
            const siteUrls = formData.getAll("site_url");
            for (let i = 0; i < siteUrls.length; i++) {
                if (siteUrls[i].trim()) {
                    sites.push({
                        title: siteTitles[i] ? siteTitles[i].trim() : "",
                        url: siteUrls[i].trim()
                    });
                }
            }
            properties["sites"] = sites;
        }

        if (type === "bookmarks") {
            const links = [];
            const linkTitles = formData.getAll("link_title");
            const linkUrls = formData.getAll("link_url");
            for (let i = 0; i < linkUrls.length; i++) {
                if (linkUrls[i].trim()) {
                    links.push({
                        title: linkTitles[i] ? linkTitles[i].trim() : "",
                        url: linkUrls[i].trim()
                    });
                }
            }
            properties["groups"] = [{
                title: formData.get("group_title") ? formData.get("group_title").trim() : "Links",
                links: links
            }];
        }

        const nestedIdx = nestedIdxVal !== "" ? parseInt(nestedIdxVal, 10) : undefined;
        
        const response = await fetch("/api/widgets/update", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                page: pageData.slug,
                column: nestedIdx !== undefined ? `${col}:${idx}` : col.toString(),
                widget: nestedIdx !== undefined ? nestedIdx : parseInt(idx, 10),
                properties: properties
            })
        });

        if (response.ok) {
            hideModal();
            showToast("Widget updated successfully", "success");
            await refreshPageContentsLive();
        } else {
            const err = await response.text();
            showToast("Failed to update widget: " + err, "error");
        }
    });
}

// ----------------------------------------------------
// Page Initialization
// ----------------------------------------------------

async function setupPage() {
    const pageElement = document.getElementById("page");

    try {
        const pageContents = await fetchPageContents(pageData.slug);
        pageElement.innerHTML = pageContents;
    } catch (e) {
        console.error("[Init] Failed to load page:", e);
        pageElement.innerHTML = '<div style="text-align:center; padding:40px; opacity:0.6;">Failed to load page content. Please refresh.</div>';
        return;
    }

    setTimeout(() => {
        document.body.classList.add("animate-element-transition");
    }, 150);

    setTimeout(setupLazyImages, 5);
    setupCarousels();
    setupDynamicRelativeTime();

    setupHeaderControls();
    setupAddWidgetModal();
    setupEditWidgetModal();
    setupSettingsMenu();
    setupClocks();

    document.addEventListener("keydown", (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === "z") {
            if (document.body.classList.contains("layout-edit-mode")) {
                e.preventDefault();
                undoLastLayoutChange();
            }
        }
    });

    setupSpotifyControls();
    setupWebSockets();
}

if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", setupPage);
} else {
    setupPage();
}

// ----------------------------------------------------
// Pointer Events-based Drag and Drop (works on touch + mouse)
// ----------------------------------------------------

function startPointerDrag(e, widget) {
    if (!document.body.classList.contains("layout-edit-mode")) return;
    if (draggedWidget) return;
    e.preventDefault();

    dragPointerId = e.pointerId;
    draggedWidget = widget;
    document.body.classList.add("is-dragging");

    const ghost = widget.cloneNode(true);
    ghost.classList.add("widget-drag-ghost");
    ghost.style.width = widget.offsetWidth + "px";
    document.body.appendChild(ghost);
    dragGhost = ghost;

    const rect = widget.getBoundingClientRect();
    dragOffsetX = e.clientX - rect.left;
    dragOffsetY = e.clientY - rect.top;

    ghost.style.left = (e.clientX - dragOffsetX) + "px";
    ghost.style.top = (e.clientY - dragOffsetY) + "px";

    widget.classList.add("dragging");

    placeholder = document.createElement("div");
    placeholder.className = "widget-placeholder";
    widget.parentNode.insertBefore(placeholder, widget);
}

function cancelPointerDrag() {
    if (!draggedWidget) return;
    document.body.classList.remove("is-dragging");
    document.querySelectorAll(".page-column.drop-active, .page-column-head.drop-active, .widget-group.drop-active").forEach(function(el) { el.classList.remove("drop-active"); });
    if (placeholder && placeholder.parentNode) {
        placeholder.parentNode.insertBefore(draggedWidget, placeholder);
        placeholder.remove();
    }
    draggedWidget.classList.remove("dragging");
    if (dragGhost) { dragGhost.remove(); dragGhost = null; }
    draggedWidget = null;
    placeholder = null;
    dragPointerId = -1;
}

document.addEventListener("pointermove", function(e) {
    if (!draggedWidget || !dragGhost) return;
    if (e.pointerId !== dragPointerId) return;
    e.preventDefault();

    dragGhost.style.left = (e.clientX - dragOffsetX) + "px";
    dragGhost.style.top = (e.clientY - dragOffsetY) + "px";

    var scrollThreshold = 60;
    var scrollSpeed = 8;
    if (e.clientY < scrollThreshold && e.clientY > 0) {
        window.scrollBy(0, -scrollSpeed);
    } else if (e.clientY > window.innerHeight - scrollThreshold && e.clientY < window.innerHeight) {
        window.scrollBy(0, scrollSpeed);
    }

    dragGhost.style.display = "none";
    var elementUnder = document.elementFromPoint(e.clientX, e.clientY);
    dragGhost.style.display = "";

    if (!elementUnder) return;

    document.querySelectorAll(".page-column.drop-active, .page-column-head.drop-active, .widget-group.drop-active").forEach(function(el) { el.classList.remove("drop-active"); });

    var dropZone = elementUnder.closest(".widget-group, .page-column, .page-column-head");
    if (!dropZone) return;
    if (draggedWidget.contains(dropZone)) return;

    dropZone.classList.add("drop-active");
    var afterElement = getDragAfterElement(dropZone, e.clientY);
    if (afterElement == null) {
        dropZone.appendChild(placeholder);
    } else {
        dropZone.insertBefore(placeholder, afterElement);
    }
});

document.addEventListener("pointerup", function(e) {
    if (!draggedWidget) return;
    if (e.pointerId !== dragPointerId) return;

    document.body.classList.remove("is-dragging");
    document.querySelectorAll(".page-column.drop-active, .page-column-head.drop-active, .widget-group.drop-active").forEach(function(el) { el.classList.remove("drop-active"); });

    if (placeholder && placeholder.parentNode) {
        placeholder.parentNode.insertBefore(draggedWidget, placeholder);
        placeholder.remove();
    }

    draggedWidget.classList.remove("dragging");

    if (dragGhost) { dragGhost.remove(); dragGhost = null; }

    draggedWidget = null;
    placeholder = null;
    dragPointerId = -1;

    pushLayoutHistory();
});

document.addEventListener("pointercancel", function() {
    cancelPointerDrag();
});
