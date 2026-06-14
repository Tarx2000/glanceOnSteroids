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
let dragAfterCache = new Map();
let hoveredTabSlug = null;
let hoverTabTimer = null;

const editPageStates = new Map();
let currentEditPageSlug = null;

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

function snapshotCurrentEditPage() {
    if (!currentEditPageSlug) return;
    const page = document.getElementById("page");
    editPageStates.set(currentEditPageSlug, {
        html: page ? page.innerHTML : "",
        layoutHistory: layoutHistory.slice(),
        historyIndex: historyIndex,
        spacingModified: spacingModified,
        originalSpacing: originalSpacing ? Object.assign({}, originalSpacing) : null
    });
}

function restoreEditPageState(slug) {
    const state = editPageStates.get(slug);
    const page = document.getElementById("page");
    if (state && page) {
        page.innerHTML = state.html;
        layoutHistory = state.layoutHistory.slice();
        historyIndex = state.historyIndex;
        spacingModified = state.spacingModified;
        originalSpacing = state.originalSpacing ? Object.assign({}, state.originalSpacing) : null;
    } else {
        layoutHistory = [];
        historyIndex = -1;
        spacingModified = false;
        originalSpacing = null;
    }
    currentEditPageSlug = slug;
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
    if (window.carouselObservers) {
        window.carouselObservers.forEach(obs => obs.disconnect());
    }
    window.carouselObservers = [];
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
        window.carouselObservers.push(resizeObserver);

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
        if (elements.length === 0) {
            if (window.relativeTimeTimeout) {
                clearInterval(window.relativeTimeTimeout);
                window.relativeTimeTimeout = null;
                window.relativeTimeIntervalInitialized = false;
            }
            return;
        }
        updateRelativeTimeForElements(elements);
        window.lastRelativeUpdateTime = Date.now();
    };

    if (!window.relativeTimeIntervalInitialized) {
        const elements = document.querySelectorAll("[data-dynamic-relative-time]");
        if (elements.length === 0) return;

        window.relativeTimeIntervalInitialized = true;
        updateElementsAndTimestamp();

        const updateInterval = 60 * 1000;
        window.lastRelativeUpdateTime = Date.now();

        const scheduleRepeatingUpdate = () => setInterval(updateElementsAndTimestamp, updateInterval);

        if (document.hidden === undefined) {
            window.relativeTimeTimeout = scheduleRepeatingUpdate();
            return;
        }

        window.relativeTimeTimeout = scheduleRepeatingUpdate();

        document.addEventListener("visibilitychange", () => {
            if (document.hidden) {
                if (window.relativeTimeTimeout) {
                    clearInterval(window.relativeTimeTimeout);
                    window.relativeTimeTimeout = null;
                }
                return;
            }

            const delta = Date.now() - window.lastRelativeUpdateTime;

            if (delta >= updateInterval) {
                updateElementsAndTimestamp();
                window.relativeTimeTimeout = scheduleRepeatingUpdate();
                return;
            }

            window.relativeTimeTimeout = setTimeout(() => {
                updateElementsAndTimestamp();
                window.relativeTimeTimeout = scheduleRepeatingUpdate();
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
    if (!player) {
        clearInterval(spotifyInterval);
        return;
    }

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

        // Dynamically update subtitle to reflect network connection loss if needed
        const idleSubtitle = document.querySelector("#spotify-idle-state .spotify-idle-subtitle");
        if (idleSubtitle) {
            if (data && data.connectionLost) {
                idleSubtitle.innerText = "Connection lost. Reconnecting...";
            } else {
                idleSubtitle.innerText = "No active playback found";
            }
        }
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

    // Update overlay play/pause icon on cover art
    const overlayPlay = document.getElementById("spotify-overlay-play");
    const overlayPause = document.getElementById("spotify-overlay-pause");
    if (track.is_playing) {
        if (overlayPlay) overlayPlay.style.display = "none";
        if (overlayPause) overlayPause.style.display = "block";
    } else {
        if (overlayPlay) overlayPlay.style.display = "block";
        if (overlayPause) overlayPause.style.display = "none";
    }

    // Toggle visualizer paused/playing state for smooth settle animation
    const playingIndicator = document.getElementById("spotify-playing-indicator");
    if (playingIndicator) {
        playingIndicator.classList.toggle("paused", !track.is_playing);
    }

    // Restore expanded/collapsed state from localStorage
    const layout = document.getElementById("spotify-track-layout");
    if (layout) {
        const isExpanded = localStorage.getItem("spotify_expanded") === "true";
        layout.classList.toggle("expanded", isExpanded);
    }

    startLocalSpotifyTicker(track.progress_ms, track.duration_ms, track.is_playing);
}

// Configures click and slider input events for the Spotify widget
function setupSpotifyControls() {
    document.addEventListener("click", async (e) => {
        const btnPlayPause = e.target.closest("#spotify-btn-play-pause");
        const coverContainer = e.target.closest("#spotify-cover-container");
        const infoContainer = e.target.closest("#spotify-info-container");
        const btnPrev = e.target.closest("#spotify-btn-prev");
        const btnNext = e.target.closest("#spotify-btn-next");

        // Toggle expanded/collapsed controls when clicking track info
        if (infoContainer && !e.target.closest(".spotify-control-btn") && !e.target.closest("#spotify-volume-slider")) {
            const layout = document.getElementById("spotify-track-layout");
            if (layout) {
                const willExpand = !layout.classList.contains("expanded");
                layout.classList.toggle("expanded");
                localStorage.setItem("spotify_expanded", willExpand ? "true" : "false");
            }
            return;
        }

        // Play / pause when clicking the album cover (or the explicit play/pause button)
        const playPauseTrigger = btnPlayPause || coverContainer;
        if (playPauseTrigger) {
            try {
                const iconPlay = document.getElementById("spotify-icon-play");
                const iconPause = document.getElementById("spotify-icon-pause");
                const overlayPlay = document.getElementById("spotify-overlay-play");
                const overlayPause = document.getElementById("spotify-overlay-pause");
                const isPlaying = iconPause && iconPause.style.display !== "none";

                // Optimistically toggle icons + visualizer animation state
                const indicator = document.getElementById("spotify-playing-indicator");
                if (isPlaying) {
                    if (iconPlay) iconPlay.style.display = "block";
                    if (iconPause) iconPause.style.display = "none";
                    if (overlayPlay) overlayPlay.style.display = "block";
                    if (overlayPause) overlayPause.style.display = "none";
                    if (indicator) indicator.classList.add("paused");
                } else {
                    if (iconPlay) iconPlay.style.display = "none";
                    if (iconPause) iconPause.style.display = "block";
                    if (overlayPlay) overlayPlay.style.display = "none";
                    if (overlayPause) overlayPause.style.display = "block";
                    if (indicator) indicator.classList.remove("paused");
                }

                const action = isPlaying ? "pause" : "play";
                const resp = await fetch(`/api/spotify/${action}`, { method: "POST" });
                if (!resp.ok) {
                    console.warn("[Spotify] Control action failed:", resp.status);
                    // Revert the optimistic toggling if the backend action fails
                    if (isPlaying) {
                        if (iconPlay) iconPlay.style.display = "none";
                        if (iconPause) iconPause.style.display = "block";
                        if (overlayPlay) overlayPlay.style.display = "none";
                        if (overlayPause) overlayPause.style.display = "block";
                        if (indicator) indicator.classList.remove("paused");
                    } else {
                        if (iconPlay) iconPlay.style.display = "block";
                        if (iconPause) iconPause.style.display = "none";
                        if (overlayPlay) overlayPlay.style.display = "block";
                        if (overlayPause) overlayPause.style.display = "none";
                        if (indicator) indicator.classList.add("paused");
                    }
                }
            } catch (e) {
                console.warn("[Spotify] Play/pause action error:", e);
            }
            return;
        }

        try {
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
let activeWS = null;
let isInitialWsConnection = true;
let allowEditModeRefresh = false;

function triggerLivePageRefreshDebounced() {
    clearTimeout(pageRefreshTimeout);
    pageRefreshTimeout = setTimeout(async () => {
        await refreshPageContentsLive();
    }, 200);
}

// Establishes a WebSocket connection for real-time status syncing
function setupWebSockets() {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws?page=${encodeURIComponent(pageData.slug)}`);
    activeWS = ws;

    ws.onopen = function () {
        console.log("[WS] Connection established.");
        if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: "active_page", page: pageData.slug }));
        }
        if (isInitialWsConnection) {
            isInitialWsConnection = false;
        } else {
            console.log("[WS] Reconnection: Syncing page contents for fresh updates...");
            triggerLivePageRefreshDebounced();
        }
    };

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
            } else if (msg.type === "widget_update") {
                console.log("[WS] Widget update received:", msg.data);
                if (msg.data.page === pageData.slug) {
                    refreshWidget(msg.data.col, msg.data.idx, msg.data.nested_idx);
                }
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
        // Inform Spotify widget of connection loss so it can update its UI status
        updateSpotifyWidget({
            authorized: true,
            track: null,
            connectionLost: true
        });
        setTimeout(setupWebSockets, 5000);
    };
}

// ----------------------------------------------------
// Layout Drag and Drop Editor
// ----------------------------------------------------

let layoutOriginalHTML = "";
let activeLayoutSaved = false;
let spacingModified = false;
let originalSpacing = null;

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
        currentEditPageSlug = pageData.slug;
        editPageStates.clear();
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

        document.querySelectorAll(".nav .nav-item").forEach(tab => {
            if (tab.tagName === "A" && tab.getAttribute("href")) {
                const slug = tab.getAttribute("href").replace(/^\//, "");
                if (slug && slug !== "api/settings") {
                    const pageTitle = tab.textContent.trim();
                    const delBtn = document.createElement("span");
                    delBtn.className = "tab-delete-btn";
                    delBtn.textContent = "✕";
                    delBtn.title = "Delete this page";
                    delBtn.dataset.slug = slug;
                    delBtn.dataset.pageTitle = pageTitle;
                    delBtn.addEventListener("click", async function(e) {
                        e.preventDefault();
                        e.stopPropagation();
                        const pageSlug = this.dataset.slug;
                        const pageTitle = this.dataset.pageTitle;
                        const navItems = document.querySelectorAll(".nav .nav-item[href]");
                        if (navItems.length <= 1) {
                            showToast("Cannot delete the last page", "error");
                            return;
                        }
                        if (await showConfirmModal(`Delete the "${pageTitle}" page and all its widgets?`)) {
                            try {
                                const resp = await fetch("/api/pages/delete", {
                                    method: "POST",
                                    headers: { "Content-Type": "application/json" },
                                    body: JSON.stringify({ slug: pageSlug })
                                });
                                if (resp.ok) {
                                    showToast("Page deleted", "success");
                                    window.location.reload();
                                } else {
                                    const err = await resp.text();
                                    showToast("Failed to delete page: " + err, "error");
                                }
                            } catch (err) {
                                showToast("Error: " + err.message, "error");
                            }
                        }
                    });
                    tab.appendChild(delBtn);
                }
            }
        });

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
        
        // Show spacing designer toggle
        const spacingWrapper = document.getElementById("spacing-dropdown-wrapper");
        if (spacingWrapper) spacingWrapper.style.display = "inline-block";
        loadSpacingSettings();
        
        enableWidgetsDraggability(true);

        editPageStates.set(pageData.slug, {
            html: document.getElementById("page").innerHTML,
            layoutHistory: layoutHistory.slice(),
            historyIndex: 0,
            spacingModified: false,
            originalSpacing: null
        });
    } else {
        cancelPointerDrag();
        body.classList.remove("layout-edit-mode");
        document.querySelectorAll(".tab-delete-btn").forEach(b => b.remove());
        btnEdit.style.display = "block";
        btnAdd.style.display = "none";
        btnSave.style.display = "none";
        btnCancel.style.display = "none";
        if (btnUndo) btnUndo.style.display = "none";
        if (btnAddPage) btnAddPage.style.display = "none";
        if (selectLayout) selectLayout.style.display = "none";
        
        // Spacing designer hide and reset
        const spacingWrapper = document.getElementById("spacing-dropdown-wrapper");
        if (spacingWrapper) spacingWrapper.style.display = "none";
        const spacingPanel = document.getElementById("spacing-dropdown-panel");
        if (spacingPanel) spacingPanel.style.display = "none";
        const toggleBtn = document.getElementById("btn-spacing-toggle");
        if (toggleBtn) toggleBtn.classList.remove("active");

        if (!activeLayoutSaved) {
            revertSpacingLive();
        }
        spacingModified = false;

        enableWidgetsDraggability(false);
        layoutHistory = [];
        historyIndex = -1;
        editPageStates.clear();
        currentEditPageSlug = null;
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
            w.dataset.originalPage = pageData.slug;
            if (nestedIdx !== undefined) {
                w.dataset.originalNestedIdx = nestedIdx;
            }
        }

        if (enabled) {
            w.classList.add("editable");

            const existingHandle = w.querySelector(".widget-drag-handle");
            if (!existingHandle) {
                const header = w.querySelector(".widget-header");
                if (header) {
                    const handle = document.createElement("div");
                    handle.className = "widget-drag-handle";
                    handle.title = "Drag to reorder";
                    handle.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="10" height="16" viewBox="0 0 10 16" fill="currentColor" style="display:block;"><circle cx="2.5" cy="2.5" r="1.5"/><circle cx="7.5" cy="2.5" r="1.5"/><circle cx="2.5" cy="8" r="1.5"/><circle cx="7.5" cy="8" r="1.5"/><circle cx="2.5" cy="13.5" r="1.5"/><circle cx="7.5" cy="13.5" r="1.5"/></svg>';
                    handle.addEventListener("pointerdown", function(e) { startPointerDrag(e, w); });
                    header.insertBefore(handle, header.firstChild);
                }
            } else {
                existingHandle.addEventListener("pointerdown", function(e) { startPointerDrag(e, w); });
            }

            const existingActions = w.querySelector(".widget-edit-actions");
            if (!existingActions) {
                const header = w.querySelector(".widget-header");
                if (header) {
                    const actionsDiv = document.createElement("div");
                    actionsDiv.className = "widget-edit-actions";
                    actionsDiv.style.cssText = "margin-left: auto; display: flex; align-items: center; gap: 6px; flex-shrink: 0;";

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
            } else {
                const editBtn = existingActions.querySelector(".widget-edit-btn");
                if (editBtn) {
                    editBtn.addEventListener("click", (e) => {
                        e.stopPropagation();
                        e.preventDefault();
                        const col = w.dataset.originalCol;
                        const idx = parseInt(w.dataset.originalIdx);
                        const nestedIdx = w.dataset.originalNestedIdx !== undefined ? parseInt(w.dataset.originalNestedIdx) : undefined;
                        openEditWidgetModal(col, idx, nestedIdx);
                    });
                }
                const deleteBtn = existingActions.querySelector(".widget-delete-btn");
                if (deleteBtn) {
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
    if (!dragAfterCache.has(container)) {
        const draggableElements = [...container.querySelectorAll(":scope > .widget:not(.dragging):not(.widget-placeholder)")];
        dragAfterCache.set(container, draggableElements.map(child => {
            const box = child.getBoundingClientRect();
            return { element: child, top: box.top, height: box.height };
        }));
    }

    const items = dragAfterCache.get(container);
    return items.reduce((closest, child) => {
        const offset = y - child.top - child.height / 2;
        if (offset < 0 && offset > closest.offset) {
            return { offset: offset, element: child.element };
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

    // Helper function to serialize a single widget (recursive for Group widgets)
    function serializeWidget(w, fallbackSlug) {
        const pageSlug = w.dataset.originalPage || fallbackSlug;
        let col = w.dataset.originalCol;
        let idx = w.dataset.originalIdx;
        let nestedIdx = w.dataset.originalNestedIdx;

        if (col === undefined || idx === undefined) {
            const parentCol = w.closest(".page-column, .page-column-head");
            if (parentCol) {
                if (col === undefined) {
                    col = parentCol.dataset.colIndex !== undefined ? parentCol.dataset.colIndex : (parentCol.classList.contains("page-column-head") ? "head" : "0");
                }
                if (idx === undefined) {
                    const siblings = parentCol.querySelectorAll(":scope > .widget");
                    siblings.forEach((s, i) => { if (s === w) idx = i; });
                    if (idx === undefined) idx = "0";
                }
            } else {
                if (col === undefined) col = "0";
                if (idx === undefined) idx = "0";
            }
        }

        if (w.classList.contains("widget-type-group")) {
            const nestedGroup = w.querySelector(".widget-group");
            const children = [];
            if (nestedGroup) {
                const nestedWidgets = nestedGroup.querySelectorAll(":scope > .widget");
                nestedWidgets.forEach(nw => {
                    children.push(serializeWidget(nw, fallbackSlug));
                });
            }
            const baseId = `${pageSlug}:${col}:${idx}`;
            return `${baseId}[${children.join(",")}]`;
        }
        
        if (nestedIdx !== undefined) {
            return `${pageSlug}:${col}:${idx}:${nestedIdx}`;
        }
        return `${pageSlug}:${col}:${idx}`;
    }

    function buildLayoutPayload(slug) {
        const tempDiv = document.createElement("div");
        if (slug === pageData.slug) {
            const headCol = document.querySelector(".page-column-head");
            const columns = document.querySelectorAll(".page-column");

            const payloadHead = [];
            if (headCol) {
                headCol.querySelectorAll(":scope > .widget").forEach(w => {
                    payloadHead.push(serializeWidget(w, slug));
                });
            }

            const payloadColumns = [];
            const payloadColumnSizes = [];
            columns.forEach(col => {
                const colWidgetsArr = [];
                col.querySelectorAll(":scope > .widget").forEach(w => {
                    colWidgetsArr.push(serializeWidget(w, slug));
                });
                payloadColumns.push(colWidgetsArr);
                if (col.classList.contains("page-column-full")) payloadColumnSizes.push("full");
                else if (col.classList.contains("page-column-small")) payloadColumnSizes.push("small");
            });

            return { page: slug, head: payloadHead, columns: payloadColumns, column_sizes: payloadColumnSizes };
        } else {
            const state = editPageStates.get(slug);
            if (!state) return null;
            tempDiv.innerHTML = state.html;

            const headCol = tempDiv.querySelector(".page-column-head");
            const columns = tempDiv.querySelectorAll(".page-column");

            const payloadHead = [];
            if (headCol) {
                headCol.querySelectorAll(":scope > .widget").forEach(w => {
                    payloadHead.push(serializeWidget(w, slug));
                });
            }

            const payloadColumns = [];
            const payloadColumnSizes = [];
            columns.forEach(col => {
                const colWidgetsArr = [];
                col.querySelectorAll(":scope > .widget").forEach(w => {
                    colWidgetsArr.push(serializeWidget(w, slug));
                });
                payloadColumns.push(colWidgetsArr);
                if (col.classList.contains("page-column-full")) payloadColumnSizes.push("full");
                else if (col.classList.contains("page-column-small")) payloadColumnSizes.push("small");
            });

            return { page: slug, head: payloadHead, columns: payloadColumns, column_sizes: payloadColumnSizes };
        }
    }

    // Snapshot current page before saving
    snapshotCurrentEditPage();

    activeLayoutSaved = true;

    if (spacingModified) {
        try {
            // Fetch current settings to get complete payload
            const responseSettings = await fetch("/api/settings");
            if (!responseSettings.ok) throw new Error("Settings fetch failed");
            const data = await responseSettings.json();
            
            // Get slider values
            const gapVal = document.getElementById("designer_widget_gap").value + "px";
            const vertVal = document.getElementById("designer_vertical_padding").value + "px";
            const horizVal = document.getElementById("designer_horizontal_padding").value + "px";
            const radiusVal = document.getElementById("designer_border_radius").value + "px";

            // Update theme properties
            data.theme["widget-gap"] = gapVal;
            data.theme["widget-content-vertical-padding"] = vertVal;
            data.theme["widget-content-horizontal-padding"] = horizVal;
            data.theme["border-radius"] = radiusVal;

            // Save settings
            const saveSettingsResp = await fetch("/api/settings/save", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(data)
            });
            if (!saveSettingsResp.ok) {
                const errText = await saveSettingsResp.text();
                throw new Error("Failed to save spacing: " + errText);
            }
        } catch (e) {
            showToast(e.message, "error");
            activeLayoutSaved = false;
            if (btnSave) {
                btnSave.disabled = false;
                btnSave.textContent = "Save";
                btnSave.style.opacity = "";
            }
            if (btnCancel) btnCancel.disabled = false;
            if (header) header.style.pointerEvents = "";
            return;
        }
    }

    try {
        const pageSlugs = [...editPageStates.keys()];
        const payloads = [];
        for (const slug of pageSlugs) {
            const payload = buildLayoutPayload(slug);
            if (payload) payloads.push(payload);
        }

        const batchPayload = { pages: payloads, single_page: payloads.length === 1 };

        if (payloads.length === 1) {
            const response = await fetch("/api/layout/save", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payloads[0])
            });
            if (!response.ok) {
                activeLayoutSaved = false;
                const err = await response.text();
                showToast("Failed to save layout: " + err, "error");
            } else {
                const wasSpacingModified = spacingModified;
                toggleEditMode(false);
                showToast("Layout saved successfully", "success");
                if (wasSpacingModified) {
                    window.location.reload();
                } else {
                    allowEditModeRefresh = true;
                    await refreshPageContentsLive();
                }
            }
        } else {
            const response = await fetch("/api/layout/batch-save", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(batchPayload)
            });
            if (!response.ok) {
                activeLayoutSaved = false;
                const err = await response.text();
                showToast("Failed to save layout: " + err, "error");
            } else {
                const wasSpacingModified = spacingModified;
                toggleEditMode(false);
                showToast("Layout saved successfully", "success");
                if (wasSpacingModified) {
                    window.location.reload();
                } else {
                    allowEditModeRefresh = true;
                    await refreshPageContentsLive();
                }
            }
        }
    } catch (err) {
        activeLayoutSaved = false;
        showToast("Network error saving layout: " + err.message, "error");
    }

    activeLayoutSaved = false;

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
        allowEditModeRefresh = true;
        await refreshPageContentsLive();
        if (document.body.classList.contains("layout-edit-mode")) {
            enableWidgetsDraggability(false);
            enableWidgetsDraggability(true);
            layoutHistory = [document.getElementById("page").innerHTML];
            historyIndex = 0;
            snapshotCurrentEditPage();
        }
    } else {
        const err = await response.text();
        showToast("Failed to delete widget: " + err, "error");
    }
}

// ----------------------------------------------------
// Widget Addition Modal & Form Generation
// ----------------------------------------------------

const defaultCacheDurations = {
    calendar: "10m",
    "custom-api": "1m",
    "hacker-news": "30m",
    monitor: "5m",
    neuralwatt: "10m",
    reddit: "30m",
    releases: "2h",
    repository: "1h",
    rss: "1h",
    "server-stats": "30s",
    stocks: "1h",
    markets: "1h",
    "twitch-channels": "10m",
    "twitch-top-games": "10m",
    videos: "1h",
    weather: "1h",
    mvv: "2m",
    gmail: "10m",
    hue: "1m"
};

function parseDurationToHoursMinutes(durationStr) {
    if (!durationStr) return { hours: 0, minutes: 15 };
    const matches = durationStr.match(/^(\d+)(s|m|h|d)$/);
    if (!matches) return { hours: 0, minutes: 15 };
    const val = parseInt(matches[1], 10);
    const unit = matches[2];
    let totalMinutes = 0;
    if (unit === 's') {
        totalMinutes = Math.max(1, Math.round(val / 60));
    } else if (unit === 'm') {
        totalMinutes = val;
    } else if (unit === 'h') {
        totalMinutes = val * 60;
    } else if (unit === 'd') {
        totalMinutes = val * 24 * 60;
    }
    const hours = Math.floor(totalMinutes / 60);
    const minutes = totalMinutes % 60;
    if (hours > 24) {
        return { hours: 24, minutes: 0 };
    }
    return { hours, minutes };
}

const widgetFieldTemplates = {
    calendar: `
        <div id="google-ip-warning" style="display: none; font-size: 0.8em; color: var(--color-negative); margin-bottom: 12px; line-height: 1.4; border: 1px solid var(--color-negative); padding: 8px; border-radius: 4px; background: rgba(255, 69, 58, 0.05);"></div>
        <div id="google-redirect-hint" style="font-size: 0.8em; color: var(--color-primary); margin-bottom: 12px; line-height: 1.4; border: 1px solid var(--color-primary); padding: 8px; border-radius: 4px; background: rgba(0, 0, 0, 0.2);">Redirect URI: http://localhost:8086/api/google/callback</div>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Google Client ID (Optional)</label>
        <input type="text" name="google_client_id" placeholder="Your Google Client ID" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Google Client Secret (Optional)</label>
        <input type="password" name="google_client_secret" placeholder="Your Google Client Secret" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Google Redirect URL (Optional)</label>
        <input type="url" name="google_redirect_url" placeholder="http://localhost:8086/api/google/callback" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Viewport Limit (Initial Entries)</label>
                <input type="number" name="viewport-limit" value="5" min="1" max="50" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Max Days Ahead</label>
                <input type="number" name="max-days-ahead" value="14" min="1" max="365" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Time Format</label>
        <select name="time-format" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;">
            <option value="24h">24 Hour Format</option>
            <option value="12h">12 Hour Format (AM/PM)</option>
        </select>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">First Day of Week</label>
        <select name="first-day-of-week" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;">
            <option value="monday">Monday</option>
            <option value="sunday">Sunday</option>
            <option value="tuesday">Tuesday</option>
            <option value="wednesday">Wednesday</option>
            <option value="thursday">Thursday</option>
            <option value="friday">Friday</option>
            <option value="saturday">Saturday</option>
        </select>
        <div id="google-calendars-container" style="display: none; margin-bottom: 10px;">
            <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Select Calendars</label>
            <div id="google-calendars-checkboxes" style="max-height: 150px; overflow-y: auto; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px;"></div>
        </div>
    `,
    weather: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Location</label>
        <input type="text" name="location" placeholder="e.g. London, United Kingdom" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Units</label>
                <select name="units" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="metric">Celsius (Metric)</option>
                    <option value="imperial">Fahrenheit (Imperial)</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Hour Format</label>
                <select name="hour-format" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="12h">12 Hour</option>
                    <option value="24h">24 Hour</option>
                </select>
            </div>
        </div>
        <div style="margin-bottom: 10px; display: flex; flex-direction: column; gap: 8px;">
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="hide-location" style="cursor: pointer;" />
                Hide Location Name
            </label>
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="show-area-name" style="cursor: pointer;" />
                Show State/Area Name
            </label>
        </div>
    `,
    iframe: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Source URL</label>
        <input type="url" name="source" placeholder="https://example.com" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Height (px)</label>
        <input type="number" name="height" value="300" min="50" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    reddit: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Subreddit</label>
        <input type="text" name="subreddit" placeholder="e.g. selfhosted" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Style</label>
                <select name="style" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Vertical List</option>
                    <option value="horizontal-cards">Horizontal Cards</option>
                    <option value="vertical-cards">Vertical Cards</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Sort By</label>
                <select name="sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="hot">Hot</option>
                    <option value="new">New</option>
                    <option value="top">Top</option>
                    <option value="rising">Rising</option>
                </select>
            </div>
        </div>
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Top Period (if Sort=Top)</label>
                <select name="top-period" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="day">Day</option>
                    <option value="hour">Hour</option>
                    <option value="week">Week</option>
                    <option value="month">Month</option>
                    <option value="year">Year</option>
                    <option value="all">All Time</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Extra Sort</label>
                <select name="extra-sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">None</option>
                    <option value="engagement">Engagement</option>
                </select>
            </div>
        </div>
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Limit</label>
                <input type="number" name="limit" value="15" min="1" max="500" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Collapse After</label>
                <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Search Keywords (Optional)</label>
        <input type="text" name="search" placeholder="e.g. selfhosted" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <div style="margin-bottom: 12px; display: flex; flex-direction: column; gap: 8px;">
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="show-thumbnails" style="cursor: pointer;" />
                Show Thumbnails
            </label>
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="show-flairs" style="cursor: pointer;" />
                Show Flairs
            </label>
        </div>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Comments URL Template (Optional)</label>
        <input type="text" name="comments-url-template" placeholder="https://www.reddit.com/{POST-PATH}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Request URL Template (Optional)</label>
        <input type="text" name="request-url-template" placeholder="https://proxy/{REQUEST-URL}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Proxy URL (Optional)</label>
        <input type="text" name="proxy" placeholder="http://user:pass@proxy.com:8080" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    rss: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">RSS Feed URLs</label>
        <div class="rss-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-rss-feed" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 15px;">+ Add Feed URL</button>
        <div style="display: flex; gap: 15px; margin-bottom: 12px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Appearance Style</label>
                <select name="style" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="vertical-list">Vertical List</option>
                    <option value="detailed-list">Detailed List</option>
                    <option value="horizontal-cards">Horizontal Cards</option>
                    <option value="horizontal-cards-2">Horizontal Cards 2</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Max Articles (Limit)</label>
                <input type="number" name="limit" value="25" min="1" max="500" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
        <div style="display: flex; gap: 15px; margin-bottom: 12px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Thumbnail Height (rem)</label>
                <input type="number" step="0.1" name="thumbnail-height" value="10" min="0" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Card Height (rem)</label>
                <input type="number" step="0.1" name="card-height" value="27" min="0" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
        <div style="display: flex; gap: 15px; margin-bottom: 12px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Collapse After</label>
                <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
        <div style="margin-bottom: 12px; display: flex; flex-direction: column; gap: 8px;">
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="preserve-order" style="cursor: pointer;" />
                Preserve Original Order of Feeds
            </label>
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="single-line-titles" style="cursor: pointer;" />
                Single Line Titles (Vertical List Only)
            </label>
        </div>
    `,
    stocks: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Stock Symbols & Names</label>
        <span style="display: block; font-size: 0.8em; opacity: 0.6; margin-bottom: 8px;">Data provided by Yahoo Finance</span>
        <div class="stocks-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-stock-symbol" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 12px;">+ Add Symbol</button>
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Sort By</label>
                <select name="sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default (Order defined)</option>
                    <option value="change">Change</option>
                    <option value="absolute-change">Absolute Change</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Style</label>
                <select name="style" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default List</option>
                    <option value="horizontal-cards">Horizontal Cards</option>
                </select>
            </div>
        </div>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Chart Link Template (Optional)</label>
        <input type="text" name="chart-link-template" placeholder="https://www.tradingview.com/chart/?symbol={SYMBOL}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Symbol Link Template (Optional)</label>
        <input type="text" name="symbol-link-template" placeholder="https://www.google.com/search?tbm=nws&q={SYMBOL}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    markets: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Market Symbols & Names</label>
        <span style="display: block; font-size: 0.8em; opacity: 0.6; margin-bottom: 8px;">Data provided by Yahoo Finance</span>
        <div class="stocks-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-stock-symbol" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 12px;">+ Add Symbol</button>
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Sort By</label>
                <select name="sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default (Order defined)</option>
                    <option value="change">Change</option>
                    <option value="absolute-change">Absolute Change</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Style</label>
                <select name="style" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default List</option>
                    <option value="horizontal-cards">Horizontal Cards</option>
                </select>
            </div>
        </div>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Chart Link Template (Optional)</label>
        <input type="text" name="chart-link-template" placeholder="https://www.tradingview.com/chart/?symbol={SYMBOL}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Symbol Link Template (Optional)</label>
        <input type="text" name="symbol-link-template" placeholder="https://www.google.com/search?tbm=nws&q={SYMBOL}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    videos: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">YouTube Channel IDs</label>
        <div class="videos-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-videos-channel" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 10px;">+ Add Channel ID</button>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">YouTube Playlist IDs (Optional)</label>
        <div class="videos-playlist-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-videos-playlist" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 10px;">+ Add Playlist ID</button>
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Style</label>
                <select name="style" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="horizontal-cards">Horizontal Cards</option>
                    <option value="vertical-list">Vertical List</option>
                    <option value="grid-cards">Grid Cards</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Limit</label>
                <input type="number" name="limit" value="25" min="1" max="500" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Collapse After</label>
                <input type="number" name="collapse-after" value="7" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Collapse After Rows (Grid)</label>
                <input type="number" name="collapse-after-rows" value="4" min="1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Video URL Template (Optional)</label>
        <input type="text" name="video-url-template" placeholder="https://www.youtube.com/watch?v={VIDEO-ID}" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
            <input type="checkbox" name="include-shorts" style="cursor: pointer;" />
            Include Shorts
        </label>
    `,
    "twitch-channels": `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Twitch Channel Names</label>
        <div class="twitch-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-twitch-channel" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit;">+ Add Channel</button>
        <div style="display: flex; gap: 15px; margin-top: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Sort By</label>
                <select name="sort-by" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="viewers">Viewers</option>
                    <option value="live">Live First</option>
                </select>
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Collapse After</label>
                <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
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
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Commits Limit</label>
                <input type="number" name="commits-limit" value="-1" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
    `,
    releases: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">GitHub Repositories (owner/repo)</label>
        <div class="releases-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-release-repo" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; margin-bottom: 10px;">+ Add Repository</button>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">GitHub Token (Optional)</label>
        <input type="password" name="token" placeholder="e.g. ghp_..." style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">GitLab Token (Optional)</label>
        <input type="password" name="gitlab-token" placeholder="e.g. glpat-..." style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <div style="display: flex; gap: 15px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Limit</label>
                <input type="number" name="limit" value="10" min="1" max="100" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Collapse After</label>
                <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
        <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
            <input type="checkbox" name="show-source-icon" style="cursor: pointer;" />
            Show Source Icon
        </label>
    `,
    monitor: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Monitored Sites</label>
        <div class="monitor-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-monitor-site" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; transition: opacity 0.2s;">+ Add Another Site</button>
        <div style="display: flex; gap: 15px; margin-top: 12px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Style</label>
                <select name="style" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;">
                    <option value="">Default</option>
                    <option value="compact">Compact</option>
                </select>
            </div>
        </div>
        <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
            <input type="checkbox" name="show-failing-only" style="cursor: pointer;" />
            Show Failing Only
        </label>
    `,
    bookmarks: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Group Title (Optional)</label>
        <input type="text" name="group_title" placeholder="e.g. My Links" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 12px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Group Color (HSL)</label>
        <input type="text" name="group_color" placeholder="e.g. 200 50 50" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 12px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Bookmark Links</label>
        <div class="bookmark-links" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-bookmark-link" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit; transition: opacity 0.2s;">+ Add Another Link</button>
    `,
    clock: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Hour Format</label>
        <select name="hour-format" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;">
            <option value="24h">24 Hour (e.g. 17:00)</option>
            <option value="12h">12 Hour (e.g. 5:00 PM)</option>
        </select>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Additional Timezones</label>
        <div class="timezone-items" style="margin-bottom: 10px;"></div>
        <button type="button" id="btn-add-timezone" style="padding: 6px 12px; font-size: 1.1rem; background: var(--color-background); border: 1px solid var(--color-primary); color: var(--color-primary); border-radius: 4px; cursor: pointer; font-family: inherit;">+ Add Timezone</button>
    `,
    "custom-api": `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">API URL (Optional)</label>
        <input type="url" name="url" placeholder="https://api.example.com/data" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">HTTP Method</label>
        <select name="method" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;">
            <option value="GET">GET</option>
            <option value="POST">POST</option>
            <option value="PUT">PUT</option>
            <option value="PATCH">PATCH</option>
            <option value="DELETE">DELETE</option>
            <option value="OPTIONS">OPTIONS</option>
            <option value="HEAD">HEAD</option>
        </select>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Custom Headers (Key: Value per line)</label>
        <textarea name="headers" placeholder="x-api-key: your-api-key" style="width: 100%; height: 50px; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; resize: vertical; outline: none; font-size: 0.85em; margin-bottom: 10px;"></textarea>
        <div style="margin-bottom: 12px; display: flex; flex-direction: column; gap: 8px;">
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="frameless" style="cursor: pointer;" />
                Frameless (remove border/padding)
            </label>
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="allow-insecure" style="cursor: pointer;" />
                Allow Insecure Certificates
            </label>
            <label style="display: flex; align-items: center; gap: 8px; font-size: 0.9em; opacity: 0.85; cursor: pointer; user-select: none;">
                <input type="checkbox" name="skip-json-validation" style="cursor: pointer;" />
                Skip JSON Validation
            </label>
        </div>
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
        <div style="display: flex; gap: 15px; margin-bottom: 10px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Post Limit</label>
                <input type="number" name="limit" value="15" min="1" max="100" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Collapse After</label>
                <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
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
        <div style="display: flex; gap: 15px;">
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Limit</label>
                <input type="number" name="limit" value="10" min="1" max="100" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div style="flex: 1;">
                <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Collapse After</label>
                <input type="number" name="collapse-after" value="5" min="-1" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
        </div>
    `,
    neuralwatt: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">NeuralWatt API Key</label>
        <input type="password" name="api-key" placeholder="sk-..." required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Update Interval (minutes)</label>
        <input type="number" name="update-interval" value="15" min="1" max="1440" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    "server-stats": `
        <p style="font-size: 0.85em; opacity: 0.7; margin-bottom: 10px;">Zeigt CPU, RAM, Disk, Docker und Uptime des Servers an.</p>
    `,
    group: `
        <p style="font-size: 0.85em; opacity: 0.7; margin-bottom: 10px;">Group widgets together using tabs. Add nested widgets via the layout editor after creating this group.</p>
    `,
    mvv: `
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Haltestellensuche (München)</label>
        <div style="display: flex; gap: 8px; margin-bottom: 12px;">
            <input type="text" id="mvv-search-input" placeholder="z. B. Marienplatz" style="flex: 1; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            <button type="button" id="btn-mvv-search" style="padding: 8px 14px; background: var(--color-primary); border: none; border-radius: 4px; color: #fff; font-weight: bold; cursor: pointer; font-family: inherit;">Suchen</button>
        </div>
        <div id="mvv-search-results" style="display: none; margin-bottom: 12px;">
            <label style="display: block; margin-bottom: 5px; font-size: 0.85em; opacity: 0.85;">Suchergebnisse</label>
            <select id="mvv-results-select" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;"></select>
        </div>
        <input type="hidden" name="station-id" id="mvv-station-id-hidden" />
        <input type="hidden" name="station-name" id="mvv-station-name-hidden" />
        
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Anzahl Abfahrten</label>
        <input type="number" name="limit" value="4" min="1" max="20" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 12px;" />
        
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Verkehrsmittel filter</label>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px;">
            <label style="display: flex; align-items: center; gap: 6px; font-size: 0.85em; cursor: pointer;"><input type="checkbox" name="show-sbahn" checked /> S-Bahn</label>
            <label style="display: flex; align-items: center; gap: 6px; font-size: 0.85em; cursor: pointer;"><input type="checkbox" name="show-ubahn" checked /> U-Bahn</label>
            <label style="display: flex; align-items: center; gap: 6px; font-size: 0.85em; cursor: pointer;"><input type="checkbox" name="show-bus" checked /> Bus</label>
            <label style="display: flex; align-items: center; gap: 6px; font-size: 0.85em; cursor: pointer;"><input type="checkbox" name="show-tram" checked /> Tram</label>
        </div>
    `,
    gmail: `
        <div style="font-size: 0.8em; color: var(--color-primary); margin-bottom: 12px; line-height: 1.4; border: 1px solid var(--color-primary); padding: 8px; border-radius: 4px; background: rgba(0, 0, 0, 0.2);" class="gmail-redirect-hint">Redirect URI: http://localhost:8086/api/google/callback</div>
        <p style="font-size: 0.85em; opacity: 0.7; margin-bottom: 12px;">Nutzt die globalen Google OAuth-Zugangsdaten. Falls noch nicht geschehen, trage diese unten ein.</p>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Google Client ID (Optional)</label>
        <input type="text" name="google_client_id" placeholder="Google Client ID" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Google Client Secret (Optional)</label>
        <input type="password" name="google_client_secret" placeholder="Google Client Secret" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Google Redirect URL (Optional)</label>
        <input type="url" name="google_redirect_url" placeholder="http://localhost:8086/api/google/callback" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
    `,
    hue: `
        <div style="font-size: 0.8em; color: var(--color-primary); margin-bottom: 12px; line-height: 1.4; border: 1px solid var(--color-primary); padding: 8px; border-radius: 4px; background: rgba(0, 0, 0, 0.2);" class="hue-redirect-hint">Redirect URI: http://localhost:8086/api/hue/callback</div>
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Philips Hue Client ID</label>
        <input type="text" name="hue_client_id" placeholder="Your Client ID" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Philips Hue Client Secret</label>
        <input type="password" name="hue_client_secret" placeholder="Your Client Secret" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 10px;" />
        <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Philips Hue Redirect URL</label>
        <input type="url" name="hue_redirect_url" placeholder="http://localhost:8086/api/hue/callback" style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none; margin-bottom: 15px;" />
        
        <div id="hue-pairing-container" style="display: none; margin-bottom: 15px;">
            <a href="/api/hue/login" id="hue-login-link" style="display: inline-block; padding: 8px 12px; background: #ffa200; color: #000; font-weight: bold; border-radius: 4px; text-decoration: none; font-size: 0.85em; text-align: center;">Verbindung herstellen (OAuth + Pairing)</a>
        </div>
        
        <div id="hue-resources-container" style="display: none; margin-bottom: 10px;">
            <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Räume / Lampen / Szenen auswählen</label>
            <div id="hue-resources-checkboxes" style="max-height: 200px; overflow-y: auto; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; display: flex; flex-direction: column; gap: 6px;"></div>
        </div>
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

        if (defaultCacheDurations[type]) {
            const cacheWrapper = document.createElement("div");
            cacheWrapper.style.cssText = "margin-bottom: 15px; padding-bottom: 10px; border-bottom: 1px dashed var(--color-separator);";
            cacheWrapper.innerHTML = `
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Update Interval</label>
                <div style="display: flex; gap: 10px; align-items: center;">
                    <div style="flex: 1; display: flex; align-items: center; gap: 5px;">
                        <input type="number" name="cache-hours" min="0" max="24" value="0" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
                        <span style="font-size: 0.9em; opacity: 0.7;">h</span>
                    </div>
                    <div style="flex: 1; display: flex; align-items: center; gap: 5px;">
                        <input type="number" name="cache-minutes" min="0" max="59" value="15" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
                        <span style="font-size: 0.9em; opacity: 0.7;">m</span>
                    </div>
                </div>
            `;
            fieldsContainer.insertBefore(cacheWrapper, fieldsContainer.firstChild);
            
            const durationObj = parseDurationToHoursMinutes(defaultCacheDurations[type]);
            cacheWrapper.querySelector('[name="cache-hours"]').value = durationObj.hours;
            cacheWrapper.querySelector('[name="cache-minutes"]').value = durationObj.minutes;
        }

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
        if (type === "calendar") {
            initGoogleCalendarFields(fieldsContainer);
        } else if (type === "mvv") {
            initMvvFields(fieldsContainer);
        } else if (type === "gmail") {
            const hint = fieldsContainer.querySelector(".gmail-redirect-hint");
            if (hint) hint.textContent = `Redirect URI: ${window.location.origin}/api/google/callback`;
        } else if (type === "hue") {
            const hint = fieldsContainer.querySelector(".hue-redirect-hint");
            if (hint) hint.textContent = `Redirect URI: ${window.location.origin}/api/hue/callback`;
            const loginLink = fieldsContainer.querySelector("#hue-login-link");
            if (loginLink) loginLink.href = `/api/hue/login`;
            initHueFields(fieldsContainer);
        }
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
            if (key === "hide-title" || key === "cache-hours" || key === "cache-minutes") return;
            if (key === "feeds" || key === "symbols" || key === "channels" || key === "repositories") {
                // Obsolete comma-separated keys; skipped to avoid noise
            } else if (key === "height" || key === "update-interval" || key === "limit" || key === "collapse-after" || key === "pull-requests-limit" || key === "issues-limit" || key === "viewport-limit" || key === "max-days-ahead" || key === "commits-limit" || key === "collapse-after-rows") {
                properties[key] = parseInt(value, 10);
            } else if (key === "thumbnail-height" || key === "card-height") {
                properties[key] = parseFloat(value);
            } else if (key === "site_title" || key === "site_url" || key === "link_title" || key === "link_url" || key === "group_title") {
                // Handled separately below
            } else if (key === "rss_url" || key === "rss_title" || key === "rss_hide_categories" || key === "rss_hide_description" || key === "rss_limit" || key === "rss_item_link_prefix" || key === "rss_headers" || key === "stocks_symbol" || key === "stocks_name" || key === "videos_channel" || key === "videos_playlist" || key === "twitch_channel" || key === "repo_name" || key === "release_repo_name" || key === "twitch_exclude" || key === "google_calendar_id" || key === "timezone_id" || key === "timezone_label" || key === "monitor_site_check_url" || key === "monitor_site_icon" || key === "monitor_site_timeout" || key === "monitor_site_alt_status") {
                // Handled separately below
            } else {
                properties[key] = value;
            }
        });

        let hours = parseInt(formData.get("cache-hours") || "0", 10);
        let minutes = parseInt(formData.get("cache-minutes") || "0", 10);
        hours = Math.max(0, Math.min(24, hours));
        minutes = Math.max(0, Math.min(59, minutes));
        if (hours === 0 && minutes === 0) {
            minutes = 1;
        }
        if (defaultCacheDurations[typeSelect.value]) {
            properties["cache"] = `${hours * 60 + minutes}m`;
        }

        const hideTitleInput = form.elements["hide-title"];
        if (hideTitleInput) {
            properties["hide-title"] = hideTitleInput.checked;
        }

        const preserveOrderInput = form.elements["preserve-order"];
        if (preserveOrderInput) {
            properties["preserve-order"] = preserveOrderInput.checked;
        }

        const singleLineTitlesInput = form.elements["single-line-titles"];
        if (singleLineTitlesInput) {
            properties["single-line-titles"] = singleLineTitlesInput.checked;
        }

        const showThumbnailsInput = form.elements["show-thumbnails"];
        if (showThumbnailsInput) {
            properties["show-thumbnails"] = showThumbnailsInput.checked;
        }
        const showFlairsInput = form.elements["show-flairs"];
        if (showFlairsInput) {
            properties["show-flairs"] = showFlairsInput.checked;
        }
        const includeShortsInput = form.elements["include-shorts"];
        if (includeShortsInput) {
            properties["include-shorts"] = includeShortsInput.checked;
        }
        const showSourceIconInput = form.elements["show-source-icon"];
        if (showSourceIconInput) {
            properties["show-source-icon"] = showSourceIconInput.checked;
        }
        const showFailingOnlyInput = form.elements["show-failing-only"];
        if (showFailingOnlyInput) {
            properties["show-failing-only"] = showFailingOnlyInput.checked;
        }
        const hideLocationInput = form.elements["hide-location"];
        if (hideLocationInput) {
            properties["hide-location"] = hideLocationInput.checked;
        }
        const showAreaNameInput = form.elements["show-area-name"];
        if (showAreaNameInput) {
            properties["show-area-name"] = showAreaNameInput.checked;
        }
        const framelessInput = form.elements["frameless"];
        if (framelessInput) {
            properties["frameless"] = framelessInput.checked;
        }
        const allowInsecureInput = form.elements["allow-insecure"];
        if (allowInsecureInput) {
            properties["allow-insecure"] = allowInsecureInput.checked;
        }
        const skipJsonValidationInput = form.elements["skip-json-validation"];
        if (skipJsonValidationInput) {
            properties["skip-json-validation"] = skipJsonValidationInput.checked;
        }
        const hideSwapInput = form.elements["hide-swap"];
        if (hideSwapInput) {
            properties["hide-swap"] = hideSwapInput.checked;
        }

        const type = typeSelect.value;

        // Dynamic lists collector for adding a new widget
        if (type === "rss") {
            const list = [];
            const items = form.querySelectorAll(".rss-item");
            items.forEach(item => {
                const urlInput = item.querySelector("[name='rss_url']");
                const url = urlInput ? urlInput.value.trim() : "";
                if (url) {
                    const titleInput = item.querySelector("[name='rss_title']");
                    const hideCategoriesCb = item.querySelector("[name='rss_hide_categories']");
                    const hideDescriptionCb = item.querySelector("[name='rss_hide_description']");
                    const limitInput = item.querySelector("[name='rss_limit']");
                    const itemLinkPrefixInput = item.querySelector("[name='rss_item_link_prefix']");
                    const headersTextarea = item.querySelector("[name='rss_headers']");
                    
                    const obj = { url: url };
                    if (titleInput && titleInput.value.trim()) {
                        obj.title = titleInput.value.trim();
                    }
                    if (hideCategoriesCb && hideCategoriesCb.checked) {
                        obj["hide-categories"] = true;
                    }
                    if (hideDescriptionCb && hideDescriptionCb.checked) {
                        obj["hide-description"] = true;
                    }
                    if (limitInput && limitInput.value.trim()) {
                        obj.limit = parseInt(limitInput.value.trim(), 10);
                    }
                    if (itemLinkPrefixInput && itemLinkPrefixInput.value.trim()) {
                        obj["item-link-prefix"] = itemLinkPrefixInput.value.trim();
                    }
                    if (headersTextarea && headersTextarea.value.trim()) {
                        const headersObj = {};
                        const lines = headersTextarea.value.trim().split("\n");
                        lines.forEach(line => {
                            const colonIdx = line.indexOf(":");
                            if (colonIdx !== -1) {
                                const k = line.substring(0, colonIdx).trim();
                                const v = line.substring(colonIdx + 1).trim();
                                if (k && v) {
                                    headersObj[k] = v;
                                }
                            }
                        });
                        if (Object.keys(headersObj).length > 0) {
                            obj.headers = headersObj;
                        }
                    }
                    list.push(obj);
                }
            });
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
            properties["playlists"] = formData.getAll("videos_playlist").map(s => s.trim()).filter(Boolean);
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

        if (type === "custom-api") {
            const headersTextarea = form.querySelector("[name='headers']");
            if (headersTextarea && headersTextarea.value.trim()) {
                const headersObj = {};
                const lines = headersTextarea.value.trim().split("\n");
                lines.forEach(line => {
                    const colonIdx = line.indexOf(":");
                    if (colonIdx !== -1) {
                        const k = line.substring(0, colonIdx).trim();
                        const v = line.substring(colonIdx + 1).trim();
                        if (k && v) {
                            headersObj[k] = v;
                        }
                    }
                });
                if (Object.keys(headersObj).length > 0) {
                    properties["headers"] = headersObj;
                }
            }
        }

        // Special handling for clock timezones
        if (type === "clock") {
            const tzItems = form.querySelectorAll(".timezone-item");
            const timezones = [];
            tzItems.forEach(item => {
                const idInput = item.querySelector("[name='timezone_id']");
                const labelInput = item.querySelector("[name='timezone_label']");
                if (idInput && idInput.value.trim()) {
                    const tz = { timezone: idInput.value.trim() };
                    if (labelInput && labelInput.value.trim()) {
                        tz.label = labelInput.value.trim();
                    }
                    timezones.push(tz);
                }
            });
            if (timezones.length > 0) {
                properties["timezones"] = timezones;
            }
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
            const groupObj = {
                title: formData.get("group_title") ? formData.get("group_title").trim() : "Links",
                links: links
            };
            const groupColor = formData.get("group_color");
            if (groupColor && groupColor.trim()) {
                groupObj.color = groupColor.trim();
            }
            properties["groups"] = [groupObj];
        }
        if (type === "calendar") {
            properties["calendars"] = Array.from(formData.getAll("google_calendar_id")).filter(Boolean);
        }
        if (type === "mvv") {
            properties["show-sbahn"] = form.elements["show-sbahn"] ? form.elements["show-sbahn"].checked : true;
            properties["show-ubahn"] = form.elements["show-ubahn"] ? form.elements["show-ubahn"].checked : true;
            properties["show-bus"] = form.elements["show-bus"] ? form.elements["show-bus"].checked : true;
            properties["show-tram"] = form.elements["show-tram"] ? form.elements["show-tram"].checked : true;
        }
        if (type === "hue") {
            properties["rooms"] = Array.from(formData.getAll("hue_room")).filter(Boolean);
            properties["lights"] = Array.from(formData.getAll("hue_light")).filter(Boolean);
            properties["scenes"] = Array.from(formData.getAll("hue_scene")).filter(Boolean);
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
            allowEditModeRefresh = true;
            await refreshPageContentsLive();
            if (document.body.classList.contains("layout-edit-mode")) {
                enableWidgetsDraggability(false);
                enableWidgetsDraggability(true);
                layoutHistory = [document.getElementById("page").innerHTML];
                historyIndex = 0;
                snapshotCurrentEditPage();
            }
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
        allowEditModeRefresh = true;
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
            
            // 1. Gather all current widgets in order (from columns only, not head)
            const allWidgets = [];
            const columns = document.querySelectorAll(".page-column");
            columns.forEach(col => {
                const colWidgets = col.querySelectorAll(":scope > .widget");
                colWidgets.forEach(w => {
                    allWidgets.push(w);
                });
            });

            // 2. Detach widgets before clearing to preserve DOM nodes
            allWidgets.forEach(w => {
                if (w.parentNode) w.parentNode.removeChild(w);
            });

            // 3. Clear page columns container
            const pageColumnsContainer = document.querySelector(".page-columns");
            if (!pageColumnsContainer) return;
            pageColumnsContainer.innerHTML = "";

            // 4. Create new columns and distribute widgets
            newSizes.forEach((size, idx) => {
                const colDiv = document.createElement("div");
                colDiv.className = `page-column page-column-${size}`;
                colDiv.dataset.colIndex = idx;
                pageColumnsContainer.appendChild(colDiv);
            });

            // 5. Distribute widgets evenly across the new columns
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

// ==========================================================
// Spacing & Density Visual Designer popover logic
// ==========================================================

function applySpacingLive(cssVar, val) {
    if (val) {
        document.documentElement.style.setProperty(cssVar, val);
    } else {
        document.documentElement.style.removeProperty(cssVar);
    }
}

function revertSpacingLive() {
    if (!originalSpacing) return;
    applySpacingLive("--widget-gap", originalSpacing["theme_widget_gap"]);
    applySpacingLive("--widget-content-vertical-padding", originalSpacing["theme_widget_vertical_padding"]);
    applySpacingLive("--widget-content-horizontal-padding", originalSpacing["theme_widget_horizontal_padding"]);
    applySpacingLive("--border-radius", originalSpacing["theme_border_radius"]);
}

function updatePresetButtonsActiveState() {
    const designerBar = document.getElementById("spacing-dropdown-panel");
    const designerGap = document.getElementById("designer_widget_gap");
    const designerVert = document.getElementById("designer_vertical_padding");
    const designerHoriz = document.getElementById("designer_horizontal_padding");
    const designerRadius = document.getElementById("designer_border_radius");

    if (!designerBar || !designerGap || !designerVert || !designerHoriz || !designerRadius) return;

    const current = {
        gap: parseInt(designerGap.value, 10),
        vertical: parseInt(designerVert.value, 10),
        horizontal: parseInt(designerHoriz.value, 10),
        radius: parseInt(designerRadius.value, 10)
    };

    const spacingPresets = {
        compact: { gap: 12, vertical: 8, horizontal: 10, radius: 4 },
        default: { gap: 25, vertical: 15, horizontal: 17, radius: 5 },
        relaxed: { gap: 40, vertical: 24, horizontal: 28, radius: 8 }
    };

    let matchedPreset = null;
    for (const [name, values] of Object.entries(spacingPresets)) {
        if (current.gap === values.gap &&
            current.vertical === values.vertical &&
            current.horizontal === values.horizontal &&
            current.radius === values.radius) {
            matchedPreset = name;
            break;
        }
    }

    const presetBtns = designerBar.querySelectorAll(".preset-btn");
    presetBtns.forEach(btn => {
        if (btn.dataset.preset === matchedPreset) {
            btn.classList.add("active");
        } else {
            btn.classList.remove("active");
        }
    });
}

async function loadSpacingSettings() {
    try {
        const response = await fetch("/api/settings");
        if (!response.ok) throw new Error("Status " + response.status);
        const data = await response.json();
        
        // Cache original values to revert if cancelled
        originalSpacing = {
            "theme_widget_gap": (data.theme && data.theme["widget-gap"]) || "",
            "theme_widget_vertical_padding": (data.theme && data.theme["widget-content-vertical-padding"]) || "",
            "theme_widget_horizontal_padding": (data.theme && data.theme["widget-content-horizontal-padding"]) || "",
            "theme_border_radius": (data.theme && data.theme["border-radius"]) || ""
        };

        const parsePxValue = (val, fallback) => {
            if (!val) return fallback;
            const num = parseInt(val, 10);
            return isNaN(num) ? fallback : num;
        };

        const gapVal = parsePxValue(originalSpacing["theme_widget_gap"], 25);
        const vertVal = parsePxValue(originalSpacing["theme_widget_vertical_padding"], 15);
        const horizVal = parsePxValue(originalSpacing["theme_widget_horizontal_padding"], 17);
        const radiusVal = parsePxValue(originalSpacing["theme_border_radius"], 5);

        const designerGap = document.getElementById("designer_widget_gap");
        const designerGapVal = document.getElementById("designer_widget_gap_val");
        const designerVert = document.getElementById("designer_vertical_padding");
        const designerVertVal = document.getElementById("designer_vertical_padding_val");
        const designerHoriz = document.getElementById("designer_horizontal_padding");
        const designerHorizVal = document.getElementById("designer_horizontal_padding_val");
        const designerRadius = document.getElementById("designer_border_radius");
        const designerRadiusVal = document.getElementById("designer_border_radius_val");

        if (designerGap && designerGapVal) {
            designerGap.value = gapVal;
            designerGapVal.textContent = gapVal + "px";
        }
        if (designerVert && designerVertVal) {
            designerVert.value = vertVal;
            designerVertVal.textContent = vertVal + "px";
        }
        if (designerHoriz && designerHorizVal) {
            designerHoriz.value = horizVal;
            designerHorizVal.textContent = horizVal + "px";
        }
        if (designerRadius && designerRadiusVal) {
            designerRadius.value = radiusVal;
            designerRadiusVal.textContent = radiusVal + "px";
        }

        updatePresetButtonsActiveState();
    } catch (e) {
        console.error("[Spacing] Failed to load spacing settings:", e);
    }
}

function setupSpacingDesigner() {
    const toggleBtn = document.getElementById("btn-spacing-toggle");
    const dropdownPanel = document.getElementById("spacing-dropdown-panel");
    
    if (toggleBtn && dropdownPanel) {
        toggleBtn.addEventListener("click", (e) => {
            e.stopPropagation();
            const isOpen = dropdownPanel.style.display === "block";
            dropdownPanel.style.display = isOpen ? "none" : "block";
            toggleBtn.classList.toggle("active", !isOpen);
        });

        document.addEventListener("click", (e) => {
            if (!dropdownPanel.contains(e.target) && e.target !== toggleBtn) {
                dropdownPanel.style.display = "none";
                toggleBtn.classList.remove("active");
            }
        });
    }

    const designerGap = document.getElementById("designer_widget_gap");
    const designerGapVal = document.getElementById("designer_widget_gap_val");
    const designerVert = document.getElementById("designer_vertical_padding");
    const designerVertVal = document.getElementById("designer_vertical_padding_val");
    const designerHoriz = document.getElementById("designer_horizontal_padding");
    const designerHorizVal = document.getElementById("designer_horizontal_padding_val");
    const designerRadius = document.getElementById("designer_border_radius");
    const designerRadiusVal = document.getElementById("designer_border_radius_val");

    const syncSliderValue = (slider, spanEl, cssVar) => {
        const val = slider.value + "px";
        spanEl.textContent = val;
        applySpacingLive(cssVar, val);
        spacingModified = true;
        updatePresetButtonsActiveState();
    };

    if (designerGap && designerGapVal) {
        designerGap.addEventListener("input", () => syncSliderValue(designerGap, designerGapVal, "--widget-gap"));
    }
    if (designerVert && designerVertVal) {
        designerVert.addEventListener("input", () => syncSliderValue(designerVert, designerVertVal, "--widget-content-vertical-padding"));
    }
    if (designerHoriz && designerHorizVal) {
        designerHoriz.addEventListener("input", () => syncSliderValue(designerHoriz, designerHorizVal, "--widget-content-horizontal-padding"));
    }
    if (designerRadius && designerRadiusVal) {
        designerRadius.addEventListener("input", () => syncSliderValue(designerRadius, designerRadiusVal, "--border-radius"));
    }

    const spacingPresets = {
        compact: { gap: 12, vertical: 8, horizontal: 10, radius: 4 },
        default: { gap: 25, vertical: 15, horizontal: 17, radius: 5 },
        relaxed: { gap: 40, vertical: 24, horizontal: 28, radius: 8 }
    };

    if (dropdownPanel) {
        dropdownPanel.querySelectorAll(".preset-btn").forEach(btn => {
            btn.addEventListener("click", () => {
                const presetName = btn.dataset.preset;
                const vals = spacingPresets[presetName];
                if (!vals) return;

                if (designerGap && designerGapVal) {
                    designerGap.value = vals.gap;
                    designerGapVal.textContent = vals.gap + "px";
                    applySpacingLive("--widget-gap", vals.gap + "px");
                }
                if (designerVert && designerVertVal) {
                    designerVert.value = vals.vertical;
                    designerVertVal.textContent = vals.vertical + "px";
                    applySpacingLive("--widget-content-vertical-padding", vals.vertical + "px");
                }
                if (designerHoriz && designerHorizVal) {
                    designerHoriz.value = vals.horizontal;
                    designerHorizVal.textContent = vals.horizontal + "px";
                    applySpacingLive("--widget-content-horizontal-padding", vals.horizontal + "px");
                }
                if (designerRadius && designerRadiusVal) {
                    designerRadius.value = vals.radius;
                    designerRadiusVal.textContent = vals.radius + "px";
                    applySpacingLive("--border-radius", vals.radius + "px");
                }

                spacingModified = true;
                updatePresetButtonsActiveState();
            });
        });
    }
}

// Updates all digital clock widgets with local time and date client-side
function setupClocks() {
    const updateClocks = () => {
        const clocks = document.querySelectorAll(".clock-widget");
        if (clocks.length === 0) {
            if (window.clockIntervalId) {
                clearInterval(window.clockIntervalId);
                window.clockIntervalId = null;
                window.clockIntervalInitialized = false;
            }
            return;
        }

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
        window.clockIntervalId = setInterval(updateClocks, 1000);
    }
}

// Traverses all widget containers on the page and dynamically sets data attributes (coordinates)
// so that the frontend can uniquely target and refresh individual widgets.
function assignDomCoordinates() {
    // 1. Head widgets
    const headZone = document.querySelector(".page-head-widgets");
    if (headZone) {
        const headWidgets = Array.from(headZone.children).filter(el => el.classList.contains("widget"));
        headWidgets.forEach((w, wIdx) => {
            w.setAttribute("data-col", "head");
            w.setAttribute("data-idx", wIdx);
            w.setAttribute("data-nested-idx", "-1");
            
            // Check for nested group widgets
            if (w.classList.contains("widget-type-group")) {
                const nested = w.querySelectorAll(".widget-group > .widget");
                nested.forEach((nw, nwIdx) => {
                    nw.setAttribute("data-col", "head");
                    nw.setAttribute("data-idx", wIdx);
                    nw.setAttribute("data-nested-idx", nwIdx);
                });
            }
        });
    }

    // 2. Column widgets
    const columns = document.querySelectorAll(".page-column");
    columns.forEach((col, colIdx) => {
        const colWidgets = Array.from(col.children).filter(el => el.classList.contains("widget"));
        colWidgets.forEach((w, wIdx) => {
            w.setAttribute("data-col", colIdx);
            w.setAttribute("data-idx", wIdx);
            w.setAttribute("data-nested-idx", "-1");
            
            // Check for nested group widgets
            if (w.classList.contains("widget-type-group")) {
                const nested = w.querySelectorAll(".widget-group > .widget");
                nested.forEach((nw, nwIdx) => {
                    nw.setAttribute("data-col", colIdx);
                    nw.setAttribute("data-idx", wIdx);
                    nw.setAttribute("data-nested-idx", nwIdx);
                });
            }
        });
    });
}

// Fetches the newly rendered HTML for a specific widget and replaces it in the DOM in-place.
async function refreshWidget(col, idx, nestedIdx) {
    let selector = `.widget[data-col="${col}"][data-idx="${idx}"]`;
    if (nestedIdx !== undefined && nestedIdx >= 0) {
        selector = `.widget[data-col="${col}"][data-idx="${idx}"] .widget[data-nested-idx="${nestedIdx}"]`;
    }
    const widgetEl = document.querySelector(selector);
    if (!widgetEl) {
        console.warn(`[Refresh] Widget not found in DOM for selector: ${selector}`);
        return;
    }

    try {
        const queryParams = new URLSearchParams({
            page: pageData.slug,
            column: col,
            widget: idx
        });
        if (nestedIdx !== undefined && nestedIdx >= 0) {
            queryParams.append("nested", nestedIdx);
        }

        const response = await fetch(`/api/widgets/render?${queryParams.toString()}`);
        if (!response.ok) {
            throw new Error(`Failed to render widget: ${response.status}`);
        }

        const newHtml = await response.text();
        
        const tempDiv = document.createElement("div");
        tempDiv.innerHTML = newHtml.trim();
        const newWidgetEl = tempDiv.firstChild;
        
        if (newWidgetEl) {
            widgetEl.replaceWith(newWidgetEl);
            
            // Re-assign coordinate data attributes to the new element
            assignDomCoordinates();

            // Re-apply Spotify widget state and redirect URI hint if the refreshed widget is the Spotify player
            const spotifyPlayer = newWidgetEl.querySelector("#spotify-player") || (newWidgetEl.id === "spotify-player" ? newWidgetEl : null);
            if (spotifyPlayer) {
                const hint = spotifyPlayer.querySelector("#spotify-redirect-hint");
                if (hint) {
                    const redirectURI = window.location.origin + "/api/spotify/callback";
                    hint.textContent = "Redirect URI: " + redirectURI;
                }
                if (lastSpotifyState) {
                    updateSpotifyWidget(lastSpotifyState);
                } else {
                    const cachedAuth = localStorage.getItem("spotify_last_auth");
                    if (cachedAuth === "true") {
                        updateSpotifyWidget({ authorized: true, track: null });
                    }
                }
            }

            // Re-run setup functions to initialize script features on the new elements
            setupLazyImages();
            setupCarousels();
            setupDynamicRelativeTime();
            setupClocks();
        }
    } catch (e) {
        console.error(`[Refresh] Failed to refresh widget ${col}:${idx}:${nestedIdx}:`, e);
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
    if (wasEditMode && !allowEditModeRefresh && historyIndex > 0) {
        return;
    }
    allowEditModeRefresh = false;
    ignoreReloadPageUntil = Date.now() + RELOAD_PAGE_IGNORE_DURATION_MS;
    
    try {
        const pageContents = await fetchPageContents(pageData.slug);
        pageElement.innerHTML = pageContents;
        assignDomCoordinates();
        
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
function addTimezoneInput(container, timezone, label) {
    timezone = timezone || "";
    label = label || "";
    const div = document.createElement("div");
    div.className = "timezone-item";
    div.style.cssText = "display: flex; gap: 8px; margin-bottom: 8px; align-items: center;";
    div.innerHTML = `
        <input type="text" name="timezone_id" placeholder="Timezone (e.g. Europe/Paris)" required value="${timezone}" style="flex: 1; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
        <input type="text" name="timezone_label" placeholder="Label (e.g. Paris)" value="${label}" style="width: 120px; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
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

function initDynamicFields(container, type, widget) {
    if (type === "rss") {
        const itemsDiv = container.querySelector(".rss-items");
        const btnAdd = container.querySelector("#btn-add-rss-feed");
        if (!itemsDiv || !btnAdd) return;

        let feeds = [];
        if (widget && widget.feeds) {
            feeds = widget.feeds.map(f => {
                if (typeof f === "object" && f !== null) {
                    return {
                        url: f.url || "",
                        title: f.title || "",
                        hideCategories: f["hide-categories"] || false,
                        hideDescription: f["hide-description"] || false,
                        limit: f.limit,
                        itemLinkPrefix: f["item-link-prefix"] || "",
                        headers: f.headers || {}
                    };
                } else {
                    return { url: f || "", title: "" };
                }
            });
        }

        itemsDiv.innerHTML = "";
        feeds.forEach(val => addRSSFeedInput(itemsDiv, val.url, val.title, val.hideCategories, val.hideDescription, val.limit, val.itemLinkPrefix, val.headers));
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
        if (itemsDiv && btnAdd) {
            const channels = (widget && widget.channels) || [];
            itemsDiv.innerHTML = "";
            channels.forEach(val => addSingleStringInput(itemsDiv, "videos_channel", "e.g. UCsBjURrPoezykLs9EqgamOA", val));
            if (channels.length === 0) addSingleStringInput(itemsDiv, "videos_channel", "e.g. UCsBjURrPoezykLs9EqgamOA", "");
            btnAdd.addEventListener("click", () => addSingleStringInput(itemsDiv, "videos_channel", "e.g. UCsBjURrPoezykLs9EqgamOA", ""));
        }
        const playlistDiv = container.querySelector(".videos-playlist-items");
        const btnAddPlaylist = container.querySelector("#btn-add-videos-playlist");
        if (playlistDiv && btnAddPlaylist) {
            const playlists = (widget && widget.playlists) || [];
            playlistDiv.innerHTML = "";
            playlists.forEach(val => addSingleStringInput(playlistDiv, "videos_playlist", "e.g. PL8mG-RkN2uTyZZ00ObwZxxoG_nJbs3qec", val));
            if (playlists.length === 0) addSingleStringInput(playlistDiv, "videos_playlist", "e.g. PL8mG-RkN2uTyZZ00ObwZxxoG_nJbs3qec", "");
            btnAddPlaylist.addEventListener("click", () => addSingleStringInput(playlistDiv, "videos_playlist", "e.g. PL8mG-RkN2uTyZZ00ObwZxxoG_nJbs3qec", ""));
        }
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
        if (itemsDiv && btnAdd) {
            const groups = (widget && widget.groups) || [];
            const links = (groups.length > 0 && groups[0].links) || [];
            itemsDiv.innerHTML = "";
            links.forEach(link => addBookmarkLinkInput(itemsDiv, link.title || "", link.url || ""));
            if (links.length === 0) addBookmarkLinkInput(itemsDiv, "", "");
            btnAdd.addEventListener("click", () => addBookmarkLinkInput(itemsDiv, "", ""));
        }
        if (widget && widget.groups && widget.groups.length > 0) {
            const colorInput = container.querySelector('[name="group_color"]');
            if (colorInput && widget.groups[0].color) {
                colorInput.value = widget.groups[0].color;
            }
        }
    }
    else if (type === "clock") {
        const itemsDiv = container.querySelector(".timezone-items");
        const btnAdd = container.querySelector("#btn-add-timezone");
        if (!itemsDiv || !btnAdd) return;

        const timezones = (widget && widget.timezones) || [];
        itemsDiv.innerHTML = "";
        timezones.forEach(tz => {
            const id = typeof tz === "object" ? (tz.timezone || "") : tz;
            const label = typeof tz === "object" ? (tz.label || "") : "";
            addTimezoneInput(itemsDiv, id, label);
        });
        if (timezones.length === 0) addTimezoneInput(itemsDiv, "", "");
        btnAdd.addEventListener("click", () => addTimezoneInput(itemsDiv, "", ""));
    }
}

/**
 * Dynamically queries Google Calendars list and renders checkbox list in settings modal.
 */
async function initGoogleCalendarFields(container, widget) {
    const hintEl = container.querySelector("#google-redirect-hint");
    if (hintEl) {
        const origin = window.location.origin;
        hintEl.textContent = `Redirect URI: ${origin}/api/google/callback`;
        
        const warningEl = container.querySelector("#google-ip-warning");
        if (warningEl) {
            if (window.location.hostname === "127.0.0.1") {
                warningEl.innerHTML = `<strong>Warning:</strong> Google OAuth does not allow raw IP addresses like <code>127.0.0.1</code> for redirect URIs. Please access the dashboard via <a href="http://localhost:8086/" style="color:var(--color-primary);text-decoration:underline;">http://localhost:8086/</a> instead to perform the authentication.`;
                warningEl.style.display = "block";
            } else {
                warningEl.style.display = "none";
            }
        }
    }

    const checkContainer = container.querySelector("#google-calendars-container");
    const checkboxesDiv = container.querySelector("#google-calendars-checkboxes");
    if (!checkContainer || !checkboxesDiv) return;

    checkboxesDiv.innerHTML = `<p style="font-size:0.85em; opacity:0.6; padding:4px;">Loading calendars...</p>`;
    checkContainer.style.display = "block";

    try {
        const res = await fetch("/api/google/calendars");
        if (res.status === 401) {
            checkboxesDiv.innerHTML = `<p style="font-size:0.85em; opacity:0.6; padding:4px;">Please authorize Google Calendar on the dashboard first.</p>`;
            return;
        }
        if (!res.ok) {
            throw new Error(await res.text());
        }
        const calendars = await res.json();
        checkboxesDiv.innerHTML = "";

        if (calendars.length === 0) {
            checkboxesDiv.innerHTML = `<p style="font-size:0.85em; opacity:0.6; padding:4px;">No calendars found.</p>`;
        } else {
            const selectedCalendars = widget ? (widget.calendars || []) : [];
            calendars.forEach(cal => {
                const checked = selectedCalendars.includes(cal.id) ? "checked" : "";
                const label = document.createElement("label");
                label.style.cssText = "display: flex; align-items: center; gap: 8px; font-size: 0.85em; margin-bottom: 6px; cursor: pointer; user-select: none; color: inherit;";
                label.innerHTML = `
                    <input type="checkbox" name="google_calendar_id" value="${cal.id}" ${checked} style="cursor: pointer;" />
                    <span style="display:inline-block; width:10px; height:10px; border-radius:50%; background-color:${cal.backgroundColor || 'var(--color-primary)'}; flex-shrink:0;"></span>
                    <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${cal.summary}</span>
                `;
                checkboxesDiv.appendChild(label);
            });
        }
    } catch (e) {
        checkboxesDiv.innerHTML = `<p style="font-size:0.85em; color:var(--color-negative); padding:4px;">Error: ${e.message}</p>`;
    }
}

function initMvvFields(container, widget) {
    const searchInput = container.querySelector("#mvv-search-input");
    const searchBtn = container.querySelector("#btn-mvv-search");
    const resultsDiv = container.querySelector("#mvv-search-results");
    const resultsSelect = container.querySelector("#mvv-results-select");
    const idHidden = container.querySelector("#mvv-station-id-hidden");
    const nameHidden = container.querySelector("#mvv-station-name-hidden");

    if (!searchInput || !searchBtn || !resultsDiv || !resultsSelect || !idHidden || !nameHidden) return;

    if (widget) {
        idHidden.value = widget["station-id"] || "";
        nameHidden.value = widget["station-name"] || "";
        searchInput.value = widget["station-name"] || "";
    }

    const performSearch = async () => {
        const query = searchInput.value.trim();
        if (!query) return;

        searchBtn.disabled = true;
        searchBtn.textContent = "Lade...";

        try {
            const res = await fetch(`/api/mvv/search?query=${encodeURIComponent(query)}`);
            if (!res.ok) throw new Error("Search failed");
            const items = await res.json();

            resultsSelect.innerHTML = "";
            if (!items || items.length === 0) {
                resultsSelect.appendChild(new Option("Keine Haltestelle gefunden", ""));
                resultsDiv.style.display = "block";
                idHidden.value = "";
                nameHidden.value = "";
            } else {
                items.forEach(item => {
                    resultsSelect.appendChild(new Option(item.name, item.id));
                });
                resultsDiv.style.display = "block";
                idHidden.value = resultsSelect.value;
                nameHidden.value = resultsSelect.options[resultsSelect.selectedIndex].text;
            }
        } catch (e) {
            console.error(e);
            resultsSelect.innerHTML = "";
            resultsSelect.appendChild(new Option("Fehler bei der Suche", ""));
            resultsDiv.style.display = "block";
        } finally {
            searchBtn.disabled = false;
            searchBtn.textContent = "Suchen";
        }
    };

    searchBtn.addEventListener("click", performSearch);
    searchInput.addEventListener("keypress", (e) => {
        if (e.key === "Enter") {
            e.preventDefault();
            performSearch();
        }
    });

    resultsSelect.addEventListener("change", () => {
        idHidden.value = resultsSelect.value;
        nameHidden.value = resultsSelect.options[resultsSelect.selectedIndex].text;
    });
}

async function initHueFields(container, widget) {
    const pairingContainer = container.querySelector("#hue-pairing-container");
    const checkContainer = container.querySelector("#hue-resources-container");
    const checkboxesDiv = container.querySelector("#hue-resources-checkboxes");

    if (!pairingContainer || !checkContainer || !checkboxesDiv) return;

    checkboxesDiv.innerHTML = `<p style="font-size:0.85em; opacity:0.6; padding:4px;">Lade Ressourcen...</p>`;
    checkContainer.style.display = "block";

    try {
        const res = await fetch("/api/hue/resources");
        if (res.status === 500) {
            checkboxesDiv.innerHTML = "";
            checkContainer.style.display = "none";
            pairingContainer.style.display = "block";
            return;
        }
        if (!res.ok) {
            throw new Error(await res.text());
        }

        const resources = await res.json();
        checkboxesDiv.innerHTML = "";
        pairingContainer.style.display = "none";

        if (resources.length === 0) {
            checkboxesDiv.innerHTML = `<p style="font-size:0.85em; opacity:0.6; padding:4px;">Keine Lampen oder Räume gefunden.</p>`;
        } else {
            const selectedRooms = widget ? (widget.rooms || []) : [];
            const selectedLights = widget ? (widget.lights || []) : [];
            const selectedScenes = widget ? (widget.scenes || []) : [];

            const rooms = resources.filter(r => r.type === "room");
            const lights = resources.filter(r => r.type === "light");
            const scenes = resources.filter(r => r.type === "scene");

            const addSection = (title, items, selectedList, inputName) => {
                if (items.length === 0) return;
                const secHeader = document.createElement("div");
                secHeader.style.cssText = "font-weight:bold; font-size:0.82em; opacity:0.8; margin-top:8px; border-bottom:1px solid rgba(255,255,255,0.08); padding-bottom:3px; margin-bottom:5px;";
                secHeader.textContent = title;
                checkboxesDiv.appendChild(secHeader);

                items.forEach(item => {
                    const checked = selectedList.includes(item.id) ? "checked" : "";
                    const label = document.createElement("label");
                    label.style.cssText = "display: flex; align-items: center; gap: 8px; font-size: 0.85em; margin-top: 4px; cursor: pointer; user-select: none; color: inherit;";
                    label.innerHTML = `
                        <input type="checkbox" name="${inputName}" value="${item.id}" ${checked} style="cursor: pointer;" />
                        <span>${item.name}</span>
                    `;
                    checkboxesDiv.appendChild(label);
                });
            };

            addSection("Räume", rooms, selectedRooms, "hue_room");
            addSection("Lampen", lights, selectedLights, "hue_light");
            addSection("Szenen", scenes, selectedScenes, "hue_scene");
        }
    } catch (e) {
        checkboxesDiv.innerHTML = "";
        checkContainer.style.display = "none";
        pairingContainer.style.display = "block";
    }
}

function setupHueControls() {
    document.addEventListener("click", async (e) => {
        const toggleBtn = e.target.closest(".hue-toggle-btn");
        const sceneBtn = e.target.closest(".hue-control-scene");

        if (toggleBtn) {
            const id = toggleBtn.getAttribute("data-hue-id");
            const rtype = toggleBtn.getAttribute("data-hue-type");
            const currentState = toggleBtn.getAttribute("data-hue-state") === "true";
            const newState = !currentState;

            toggleBtn.setAttribute("data-hue-state", newState ? "true" : "false");
            toggleBtn.textContent = newState ? "AN" : "AUS";
            toggleBtn.style.background = newState ? "var(--color-primary)" : "rgba(255,255,255,0.08)";
            toggleBtn.style.color = newState ? "#fff" : "inherit";
            if (newState) {
                toggleBtn.style.boxShadow = "0 0 10px rgba(var(--color-primary-rgb, 0, 122, 255), 0.35)";
            } else {
                toggleBtn.style.boxShadow = "none";
            }

            try {
                const res = await fetch("/api/hue/control", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ id: id, type: rtype, on: newState })
                });
                if (!res.ok) {
                    throw new Error("Hue control failed");
                }
            } catch (err) {
                console.error(err);
                showToast("Steuerung fehlgeschlagen", "error");
                toggleBtn.setAttribute("data-hue-state", currentState ? "true" : "false");
                toggleBtn.textContent = currentState ? "AN" : "AUS";
                toggleBtn.style.background = currentState ? "var(--color-primary)" : "rgba(255,255,255,0.08)";
                toggleBtn.style.color = currentState ? "#fff" : "inherit";
                if (currentState) {
                    toggleBtn.style.boxShadow = "0 0 10px rgba(var(--color-primary-rgb, 0, 122, 255), 0.35)";
                } else {
                    toggleBtn.style.boxShadow = "none";
                }
            }
            return;
        }

        if (sceneBtn) {
            const id = sceneBtn.getAttribute("data-hue-id");
            const rtype = sceneBtn.getAttribute("data-hue-type");

            sceneBtn.style.transform = "scale(0.97)";
            setTimeout(() => {
                sceneBtn.style.transform = "";
            }, 100);

            try {
                const res = await fetch("/api/hue/control", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ id: id, type: rtype, on: true })
                });
                if (!res.ok) {
                    throw new Error("Scene activation failed");
                }
                showToast("Szene aktiviert", "success");
            } catch (err) {
                console.error(err);
                showToast("Aktivierung fehlgeschlagen", "error");
            }
        }
    });
}

/**
 * Appends a single RSS Feed input card with URL, Title, and collapsible Advanced settings.
 */
function addRSSFeedInput(container, url, title, hideCategories, hideDescription, limit, itemLinkPrefix, headers) {
    url = url || "";
    title = title || "";
    hideCategories = hideCategories === true;
    hideDescription = hideDescription === true;
    limit = (limit !== undefined && limit !== null) ? limit : "";
    itemLinkPrefix = itemLinkPrefix || "";
    
    let headersStr = "";
    if (headers && typeof headers === "object") {
        headersStr = Object.entries(headers).map(([k, v]) => `${k}: ${v}`).join("\n");
    }

    const div = document.createElement("div");
    div.className = "rss-item";
    div.style.cssText = "display: flex; flex-direction: column; gap: 8px; margin-bottom: 12px; border: 1px solid var(--color-widget-content-border); padding: 12px; border-radius: 6px; background: rgba(255, 255, 255, 0.02);";
    
    div.innerHTML = `
        <div style="display: flex; gap: 8px; align-items: center;">
            <input type="url" name="rss_url" placeholder="Feed URL (e.g. https://selfh.st/rss/)" required value="${url}" style="flex: 1; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            <button type="button" class="btn-remove-dynamic-item" style="background: transparent; border: 1px solid var(--color-negative); color: var(--color-negative); border-radius: 4px; padding: 0 10px; height: 34px; font-size: 1.2rem; cursor: pointer; transition: all 0.2s ease; font-weight: bold;">×</button>
        </div>
        <div style="display: flex; justify-content: space-between; align-items: center;">
            <button type="button" class="btn-feed-advanced-toggle" style="background: none; border: none; color: var(--color-primary); cursor: pointer; font-size: 0.85em; padding: 0; display: flex; align-items: center; gap: 4px; font-family: inherit;">
                Advanced Settings <span class="arrow-indicator">▶</span>
            </button>
        </div>
        <div class="feed-advanced-panel" style="display: none; flex-direction: column; gap: 10px; border-top: 1px dashed var(--color-widget-content-border); padding-top: 10px; margin-top: 5px;">
            <div style="display: flex; gap: 10px;">
                <div style="flex: 1;">
                    <label style="display: block; margin-bottom: 4px; font-size: 0.8em; opacity: 0.85;">Title Override</label>
                    <input type="text" name="rss_title" placeholder="Feed Title Override" value="${title}" style="width: 100%; padding: 6px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
                </div>
                <div style="width: 80px;">
                    <label style="display: block; margin-bottom: 4px; font-size: 0.8em; opacity: 0.85;">Feed Limit</label>
                    <input type="number" name="rss_limit" placeholder="No limit" value="${limit}" style="width: 100%; padding: 6px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
                </div>
            </div>
            <div>
                <label style="display: block; margin-bottom: 4px; font-size: 0.8em; opacity: 0.85;">Link Prefix Override</label>
                <input type="text" name="rss_item_link_prefix" placeholder="e.g. https://domain.com" value="${itemLinkPrefix}" style="width: 100%; padding: 6px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
            </div>
            <div>
                <label style="display: block; margin-bottom: 4px; font-size: 0.8em; opacity: 0.85;">Custom Headers (Key: Value per line)</label>
                <textarea name="rss_headers" placeholder="User-Agent: Custom Agent" style="width: 100%; height: 50px; padding: 6px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; resize: vertical; outline: none; font-size: 0.85em;">${headersStr}</textarea>
            </div>
            <div style="display: flex; gap: 15px; margin-top: 2px;">
                <label style="display: flex; align-items: center; gap: 6px; font-size: 0.8em; opacity: 0.85; cursor: pointer; user-select: none;">
                    <input type="checkbox" name="rss_hide_categories" ${hideCategories ? "checked" : ""} style="cursor: pointer;" />
                    Hide Categories
                </label>
                <label style="display: flex; align-items: center; gap: 6px; font-size: 0.8em; opacity: 0.85; cursor: pointer; user-select: none;">
                    <input type="checkbox" name="rss_hide_description" ${hideDescription ? "checked" : ""} style="cursor: pointer;" />
                    Hide Description
                </label>
            </div>
        </div>
    `;
    container.appendChild(div);

    // Toggle advanced settings panel visibility
    const advancedToggle = div.querySelector(".btn-feed-advanced-toggle");
    const advancedPanel = div.querySelector(".feed-advanced-panel");
    const arrowIndicator = div.querySelector(".arrow-indicator");
    advancedToggle.addEventListener("click", () => {
        if (advancedPanel.style.display === "none") {
            advancedPanel.style.display = "flex";
            arrowIndicator.textContent = "▼";
        } else {
            advancedPanel.style.display = "none";
            arrowIndicator.textContent = "▶";
        }
    });

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

        if (defaultCacheDurations[type]) {
            const cacheWrapper = document.createElement("div");
            cacheWrapper.style.cssText = "margin-bottom: 15px; padding-bottom: 10px; border-bottom: 1px dashed var(--color-separator);";
            cacheWrapper.innerHTML = `
                <label style="display: block; margin-bottom: 5px; font-size: 0.9em; opacity: 0.85;">Update Interval</label>
                <div style="display: flex; gap: 10px; align-items: center;">
                    <div style="flex: 1; display: flex; align-items: center; gap: 5px;">
                        <input type="number" name="cache-hours" min="0" max="24" value="0" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
                        <span style="font-size: 0.9em; opacity: 0.7;">h</span>
                    </div>
                    <div style="flex: 1; display: flex; align-items: center; gap: 5px;">
                        <input type="number" name="cache-minutes" min="0" max="59" value="15" required style="width: 100%; padding: 8px; background: var(--color-background); border: 1px solid var(--color-widget-content-border); border-radius: 4px; color: inherit; font-family: inherit; outline: none;" />
                        <span style="font-size: 0.9em; opacity: 0.7;">m</span>
                    </div>
                </div>
            `;
            fieldsContainer.insertBefore(cacheWrapper, fieldsContainer.firstChild);
            
            const durationObj = parseDurationToHoursMinutes(widget.cache || defaultCacheDurations[type]);
            cacheWrapper.querySelector('[name="cache-hours"]').value = durationObj.hours;
            cacheWrapper.querySelector('[name="cache-minutes"]').value = durationObj.minutes;
        }

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
        if (type === "calendar") {
            await initGoogleCalendarFields(fieldsContainer, widget);
        } else if (type === "mvv") {
            initMvvFields(fieldsContainer, widget);
        } else if (type === "gmail") {
            const hint = fieldsContainer.querySelector(".gmail-redirect-hint");
            if (hint) hint.textContent = `Redirect URI: ${window.location.origin}/api/google/callback`;
        } else if (type === "hue") {
            const hint = fieldsContainer.querySelector(".hue-redirect-hint");
            if (hint) hint.textContent = `Redirect URI: ${window.location.origin}/api/hue/callback`;
            const loginLink = fieldsContainer.querySelector("#hue-login-link");
            if (loginLink) loginLink.href = `/api/hue/login`;
            await initHueFields(fieldsContainer, widget);
        }

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
        if (key === "type" || key === "sites" || key === "groups" || key === "feeds" || key === "symbols" || key === "markets" || key === "stocks" || key === "channels" || key === "repositories" || key === "exclude" || key === "timezones" || key === "playlists" || key === "rooms" || key === "lights" || key === "scenes") continue;
        const val = widget[key];

        const inputs = container.querySelectorAll(`[name="${key}"]`);
        inputs.forEach(input => {
            if (typeof val === "object" && val !== null) {
                if (key === "headers" && input.tagName === "TEXTAREA") {
                    input.value = Object.entries(val).map(([k, v]) => `${k}: ${v}`).join("\n");
                }
            } else if (input.type === "checkbox") {
                input.checked = val === true || val === "true" || val === "on" || val === 1;
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
            if (key === "hide-title" || key === "cache-hours" || key === "cache-minutes") return;
            if (key === "feeds" || key === "symbols" || key === "channels" || key === "repositories" || key === "exclude") {
                // Obsolete comma-separated keys; skipped to avoid noise
            } else if (key === "height" || key === "limit" || key === "collapse-after" || key === "update-interval" || key === "pull-requests-limit" || key === "issues-limit" || key === "viewport-limit" || key === "max-days-ahead" || key === "commits-limit" || key === "collapse-after-rows") {
                properties[key] = parseInt(value, 10);
            } else if (key === "thumbnail-height" || key === "card-height") {
                properties[key] = parseFloat(value);
            } else if (key === "site_title" || key === "site_url" || key === "link_title" || key === "link_url" || key === "group_title") {
                // Handled separately below
            } else if (key === "rss_url" || key === "rss_title" || key === "rss_hide_categories" || key === "rss_hide_description" || key === "rss_limit" || key === "rss_item_link_prefix" || key === "rss_headers" || key === "stocks_symbol" || key === "stocks_name" || key === "videos_channel" || key === "videos_playlist" || key === "twitch_channel" || key === "repo_name" || key === "release_repo_name" || key === "twitch_exclude" || key === "google_calendar_id" || key === "timezone_id" || key === "timezone_label" || key === "monitor_site_check_url" || key === "monitor_site_icon" || key === "monitor_site_timeout" || key === "monitor_site_alt_status") {
                // Handled separately below
            } else {
                properties[key] = value;
            }
        });

        let hours = parseInt(formData.get("cache-hours") || "0", 10);
        let minutes = parseInt(formData.get("cache-minutes") || "0", 10);
        hours = Math.max(0, Math.min(24, hours));
        minutes = Math.max(0, Math.min(59, minutes));
        if (hours === 0 && minutes === 0) {
            minutes = 1;
        }
        if (defaultCacheDurations[type]) {
            properties["cache"] = `${hours * 60 + minutes}m`;
        }

        const hideTitleInput = form.elements["hide-title"];
        if (hideTitleInput) {
            properties["hide-title"] = hideTitleInput.checked;
        }

        const preserveOrderInput = form.elements["preserve-order"];
        if (preserveOrderInput) {
            properties["preserve-order"] = preserveOrderInput.checked;
        }

        const singleLineTitlesInput = form.elements["single-line-titles"];
        if (singleLineTitlesInput) {
            properties["single-line-titles"] = singleLineTitlesInput.checked;
        }

        const showThumbnailsInput = form.elements["show-thumbnails"];
        if (showThumbnailsInput) {
            properties["show-thumbnails"] = showThumbnailsInput.checked;
        }
        const showFlairsInput = form.elements["show-flairs"];
        if (showFlairsInput) {
            properties["show-flairs"] = showFlairsInput.checked;
        }
        const includeShortsInput = form.elements["include-shorts"];
        if (includeShortsInput) {
            properties["include-shorts"] = includeShortsInput.checked;
        }
        const showSourceIconInput = form.elements["show-source-icon"];
        if (showSourceIconInput) {
            properties["show-source-icon"] = showSourceIconInput.checked;
        }
        const showFailingOnlyInput = form.elements["show-failing-only"];
        if (showFailingOnlyInput) {
            properties["show-failing-only"] = showFailingOnlyInput.checked;
        }
        const hideLocationInput = form.elements["hide-location"];
        if (hideLocationInput) {
            properties["hide-location"] = hideLocationInput.checked;
        }
        const showAreaNameInput = form.elements["show-area-name"];
        if (showAreaNameInput) {
            properties["show-area-name"] = showAreaNameInput.checked;
        }
        const framelessInput = form.elements["frameless"];
        if (framelessInput) {
            properties["frameless"] = framelessInput.checked;
        }
        const allowInsecureInput = form.elements["allow-insecure"];
        if (allowInsecureInput) {
            properties["allow-insecure"] = allowInsecureInput.checked;
        }
        const skipJsonValidationInput = form.elements["skip-json-validation"];
        if (skipJsonValidationInput) {
            properties["skip-json-validation"] = skipJsonValidationInput.checked;
        }
        const hideSwapInput = form.elements["hide-swap"];
        if (hideSwapInput) {
            properties["hide-swap"] = hideSwapInput.checked;
        }

        // Dynamic lists collector for editing a widget
        if (type === "rss") {
            const list = [];
            const items = form.querySelectorAll(".rss-item");
            items.forEach(item => {
                const urlInput = item.querySelector("[name='rss_url']");
                const url = urlInput ? urlInput.value.trim() : "";
                if (url) {
                    const titleInput = item.querySelector("[name='rss_title']");
                    const hideCategoriesCb = item.querySelector("[name='rss_hide_categories']");
                    const hideDescriptionCb = item.querySelector("[name='rss_hide_description']");
                    const limitInput = item.querySelector("[name='rss_limit']");
                    const itemLinkPrefixInput = item.querySelector("[name='rss_item_link_prefix']");
                    const headersTextarea = item.querySelector("[name='rss_headers']");
                    
                    const obj = { url: url };
                    if (titleInput && titleInput.value.trim()) {
                        obj.title = titleInput.value.trim();
                    }
                    if (hideCategoriesCb && hideCategoriesCb.checked) {
                        obj["hide-categories"] = true;
                    }
                    if (hideDescriptionCb && hideDescriptionCb.checked) {
                        obj["hide-description"] = true;
                    }
                    if (limitInput && limitInput.value.trim()) {
                        obj.limit = parseInt(limitInput.value.trim(), 10);
                    }
                    if (itemLinkPrefixInput && itemLinkPrefixInput.value.trim()) {
                        obj["item-link-prefix"] = itemLinkPrefixInput.value.trim();
                    }
                    if (headersTextarea && headersTextarea.value.trim()) {
                        const headersObj = {};
                        const lines = headersTextarea.value.trim().split("\n");
                        lines.forEach(line => {
                            const colonIdx = line.indexOf(":");
                            if (colonIdx !== -1) {
                                const k = line.substring(0, colonIdx).trim();
                                const v = line.substring(colonIdx + 1).trim();
                                if (k && v) {
                                    headersObj[k] = v;
                                }
                            }
                        });
                        if (Object.keys(headersObj).length > 0) {
                            obj.headers = headersObj;
                        }
                    }
                    list.push(obj);
                }
            });
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
            properties["playlists"] = formData.getAll("videos_playlist").map(s => s.trim()).filter(Boolean);
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

        if (type === "custom-api") {
            const headersTextarea = form.querySelector("[name='headers']");
            if (headersTextarea && headersTextarea.value.trim()) {
                const headersObj = {};
                const lines = headersTextarea.value.trim().split("\n");
                lines.forEach(line => {
                    const colonIdx = line.indexOf(":");
                    if (colonIdx !== -1) {
                        const k = line.substring(0, colonIdx).trim();
                        const v = line.substring(colonIdx + 1).trim();
                        if (k && v) {
                            headersObj[k] = v;
                        }
                    }
                });
                if (Object.keys(headersObj).length > 0) {
                    properties["headers"] = headersObj;
                }
            }
        }

        if (type === "clock") {
            const tzItems = form.querySelectorAll(".timezone-item");
            const timezones = [];
            tzItems.forEach(item => {
                const idInput = item.querySelector("[name='timezone_id']");
                const labelInput = item.querySelector("[name='timezone_label']");
                if (idInput && idInput.value.trim()) {
                    const tz = { timezone: idInput.value.trim() };
                    if (labelInput && labelInput.value.trim()) {
                        tz.label = labelInput.value.trim();
                    }
                    timezones.push(tz);
                }
            });
            if (timezones.length > 0) {
                properties["timezones"] = timezones;
            }
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
            const groupObj = {
                title: formData.get("group_title") ? formData.get("group_title").trim() : "Links",
                links: links
            };
            const groupColor = formData.get("group_color");
            if (groupColor && groupColor.trim()) {
                groupObj.color = groupColor.trim();
            }
            properties["groups"] = [groupObj];
        }
        if (type === "calendar") {
            properties["calendars"] = Array.from(formData.getAll("google_calendar_id")).filter(Boolean);
        }
        if (type === "mvv") {
            properties["show-sbahn"] = form.elements["show-sbahn"] ? form.elements["show-sbahn"].checked : true;
            properties["show-ubahn"] = form.elements["show-ubahn"] ? form.elements["show-ubahn"].checked : true;
            properties["show-bus"] = form.elements["show-bus"] ? form.elements["show-bus"].checked : true;
            properties["show-tram"] = form.elements["show-tram"] ? form.elements["show-tram"].checked : true;
        }
        if (type === "hue") {
            properties["rooms"] = Array.from(formData.getAll("hue_room")).filter(Boolean);
            properties["lights"] = Array.from(formData.getAll("hue_light")).filter(Boolean);
            properties["scenes"] = Array.from(formData.getAll("hue_scene")).filter(Boolean);
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
            allowEditModeRefresh = true;
            await refreshPageContentsLive();
            if (document.body.classList.contains("layout-edit-mode")) {
                enableWidgetsDraggability(false);
                enableWidgetsDraggability(true);
                layoutHistory = [document.getElementById("page").innerHTML];
                historyIndex = 0;
                snapshotCurrentEditPage();
            }
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
        assignDomCoordinates();
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
    setupSpacingDesigner();
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
    setupHueControls();
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

function resetHoverTab() {
    if (hoverTabTimer) {
        clearTimeout(hoverTabTimer);
        hoverTabTimer = null;
    }
    hoveredTabSlug = null;
    document.querySelectorAll(".nav .nav-item").forEach(el => el.classList.remove("tab-drag-hover"));
}

async function switchPageDynamically(targetSlug, isDragging = false) {
    resetHoverTab();
    if (isDragging && !draggedWidget) return;

    const isInEditMode = document.body.classList.contains("layout-edit-mode");

    try {
        // When dragging to another tab, remove widget + placeholder from source page
        // BEFORE snapshotting so the source page's cache doesn't contain a ghost widget
        if (isDragging) {
            if (placeholder && placeholder.parentNode) {
                placeholder.parentNode.removeChild(placeholder);
            }
            if (draggedWidget && draggedWidget.parentNode) {
                draggedWidget.parentNode.removeChild(draggedWidget);
            }
            draggedWidget.classList.remove("dragging");
        }

        // Snapshot current page state before switching (edit mode only)
        if (isInEditMode) {
            snapshotCurrentEditPage();
        }

        // Re-add dragging class for the target page
        if (isDragging && draggedWidget) {
            draggedWidget.classList.add("dragging");
            placeholder = document.createElement("div");
            placeholder.className = "widget-placeholder";
        }

        // Switch page slug
        pageData.slug = targetSlug;

        // Notify backend WebSocket of the new active page slug
        if (activeWS && activeWS.readyState === WebSocket.OPEN) {
            activeWS.send(JSON.stringify({ type: "active_page", page: targetSlug }));
        }

        // Update active tab styling in desktop header
        document.querySelectorAll(".nav .nav-item").forEach(item => {
            item.classList.remove("nav-item-current");
            const href = item.getAttribute("href");
            if (href && href.replace(/^\//, "") === targetSlug) {
                item.classList.add("nav-item-current");
            }
        });

        // Set the new HTML content — use cached state if available in edit mode
        const pageElement = document.getElementById("page");
        if (isInEditMode && editPageStates.has(targetSlug)) {
            restoreEditPageState(targetSlug);
        } else {
            const newContent = await fetchPageContents(targetSlug);
            pageElement.innerHTML = newContent;
            if (isInEditMode) {
                currentEditPageSlug = targetSlug;
                layoutHistory = [pageElement.innerHTML];
                historyIndex = 0;
            }
        }

        // Reassign column layout indexing for the new page
        assignDomCoordinates();

        if (isDragging) {
            // Place the placeholder and dragged widget in the first column of the new page
            const firstCol = pageElement.querySelector(".page-column");
            if (firstCol) {
                firstCol.appendChild(placeholder);
                placeholder.appendChild(draggedWidget);
            }
        }

        // Update URL dynamically
        window.history.pushState(null, "", "/" + targetSlug);

        // If in Edit Mode, enable widgets draggability on the new elements and update select layout
        if (isInEditMode) {
            const selectLayout = document.getElementById("select-page-layout");
            if (selectLayout) {
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
            if (!editPageStates.has(targetSlug)) {
                editPageStates.set(targetSlug, {
                    html: pageElement.innerHTML,
                    layoutHistory: layoutHistory.slice(),
                    historyIndex: 0,
                    spacingModified: false,
                    originalSpacing: null
                });
            }
            updateUndoButtonState();
        }

        // Re-initialize dynamic styles/widgets for the new page layout
        setTimeout(setupLazyImages, 5);
        setupCarousels();
        setupDynamicRelativeTime();
        setupClocks();
    } catch (e) {
        console.error("Failed to switch page dynamically:", e);
        showToast("Error switching dashboard: " + e.message, "error");
    }
}

// Intercept nav clicks in edit mode to switch pages dynamically and stay in edit mode
document.addEventListener("click", async function (e) {
    const tabLink = e.target.closest(".nav .nav-item");
    if (tabLink && tabLink.tagName === "A" && document.body.classList.contains("layout-edit-mode")) {
        const href = tabLink.getAttribute("href");
        if (href && !href.startsWith("http") && !href.startsWith("//") && !href.includes("://")) {
            const targetSlug = href.replace(/^\//, "");
            if (targetSlug && targetSlug !== "api/settings" && targetSlug !== pageData.slug) {
                e.preventDefault();
                await switchPageDynamically(targetSlug, false);
            }
        }
    }
});

function startPointerDrag(e, widget) {
    if (!document.body.classList.contains("layout-edit-mode")) return;
    if (draggedWidget) return;
    e.preventDefault();

    dragPointerId = e.pointerId;
    draggedWidget = widget;
    
    // Set the original page slug when we start dragging
    if (!widget.dataset.originalPage) {
        widget.dataset.originalPage = pageData.slug;
    }
    
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

    dragAfterCache.clear();
    widget.classList.add("dragging");

    placeholder = document.createElement("div");
    placeholder.className = "widget-placeholder";
    widget.parentNode.insertBefore(placeholder, widget);
}

function cancelPointerDrag() {
    resetHoverTab();
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
    dragAfterCache.clear();
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

    // --- Tab Switching Logic ---
    const tabLink = elementUnder.closest(".nav .nav-item"); // Must be in the header nav
    // Clear any previous tab hover style
    document.querySelectorAll(".nav .nav-item").forEach(el => el.classList.remove("tab-drag-hover"));

    if (tabLink && tabLink.tagName === "A" && !tabLink.classList.contains("nav-item-current")) {
        const href = tabLink.getAttribute("href");
        const targetSlug = href ? href.replace(/^\//, "") : "";
        
        if (targetSlug && targetSlug !== pageData.slug && targetSlug !== "api/settings") {
            tabLink.classList.add("tab-drag-hover");
            if (hoveredTabSlug !== targetSlug) {
                if (hoverTabTimer) clearTimeout(hoverTabTimer);
                hoveredTabSlug = targetSlug;
                hoverTabTimer = setTimeout(async () => {
                    await switchPageDynamically(targetSlug, true);
                }, 600); // 600ms hover duration
            }
        } else {
            resetHoverTab();
        }
    } else {
        resetHoverTab();
    }
    // ----------------------------

    document.querySelectorAll(".page-column.drop-active, .page-column-head.drop-active, .widget-group.drop-active").forEach(function(el) { el.classList.remove("drop-active"); });

    var dropZone = elementUnder.closest(".widget-group, .page-column, .page-column-head");
    if (!dropZone) return;
    if (draggedWidget.contains(dropZone)) return;

    dropZone.classList.add("drop-active");
    var afterElement = getDragAfterElement(dropZone, e.clientY);
    var shouldMove = false;
    if (afterElement == null) {
        shouldMove = placeholder.parentNode !== dropZone || placeholder.nextSibling != null;
    } else {
        shouldMove = placeholder.parentNode !== dropZone || placeholder.nextSibling !== afterElement;
    }
    if (shouldMove) {
        if (afterElement == null) {
            dropZone.appendChild(placeholder);
        } else {
            dropZone.insertBefore(placeholder, afterElement);
        }
        dragAfterCache.delete(dropZone);
    }
});

document.addEventListener("pointerup", function(e) {
    resetHoverTab();
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
    dragAfterCache.clear();

    pushLayoutHistory();
});

document.addEventListener("pointercancel", function() {
    cancelPointerDrag();
});
