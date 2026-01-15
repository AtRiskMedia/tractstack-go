package templates

import (
	"bytes"
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

var bunnyWidgetTmpl = template.Must(template.New("bunnyWidget").Parse(
	`{{define "bunnyWidget"}}<div 
        id="bunny-container-{{.SanitizedVideoID}}" 
        class="relative w-full {{.ClassNames}}"
        data-video-id="{{.VideoID}}"
        data-title="{{.Title}}"
        {{if .ChaptersJSON}}data-chapters='{{.ChaptersJSON}}'{{end}}
    >
    <div class="relative w-full aspect-video">
        <div id="bunny-loader-{{.SanitizedVideoID}}" class="absolute inset-0 flex items-center justify-center bg-gray-100 bg-opacity-75 z-10">
            <div class="flex flex-col items-center">
                <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-orange-500"></div>
                <span class="mt-2 text-sm text-gray-700">Loading video...</span>
            </div>
        </div>

        <div id="bunny-error-{{.SanitizedVideoID}}" class="hidden absolute inset-0 items-center justify-center bg-gray-100 z-10">
            <div class="text-center text-gray-800">
                <span id="bunny-error-msg-{{.SanitizedVideoID}}" class="block text-sm">Failed to load video</span>
                <button onclick="window.location.reload()" class="mt-2 text-sm text-blue-600 hover:text-orange-600">
                  Reload page
                </button>
            </div>
        </div>

        {{if .ChaptersJSON}}
        <div id="bunny-chapter-overlay-{{.SanitizedVideoID}}" class="hidden absolute top-4 right-4 z-10 rounded-md bg-black/70 px-3 py-1 text-sm text-white cursor-pointer">
            <span id="bunny-chapter-title-{{.SanitizedVideoID}}"></span>
            <a id="bunny-chapter-link-{{.SanitizedVideoID}}" class="ml-2 text-blue-300 underline" style="display: none;">[Read More]</a>
        </div>
        {{end}}
    </div>

    {{if .ChaptersJSON}}
    <div id="bunny-chapters-container-{{.SanitizedVideoID}}" class="mt-4 overflow-hidden rounded-md border border-gray-200 bg-white">
        <div id="bunny-chapters-header-{{.SanitizedVideoID}}" class="flex cursor-pointer items-center justify-between bg-gray-50 px-4 py-2">
            <h3 class="text-sm font-bold text-gray-900">Video Chapters</h3>
            <span class="toggle-text font-bold text-gray-700 text-sm">Show</span>
        </div>
        <div id="bunny-chapters-content-{{.SanitizedVideoID}}" class="hidden">
            <ul id="bunny-chapters-list-{{.SanitizedVideoID}}" class="divide-y divide-gray-100"></ul>
        </div>
    </div>
    {{end}}
    
	<script>
      (function() {
	      const VERBOSE = false;
        var sanitizedId = '{{.SanitizedVideoID}}';
	      if( VERBOSE )
          console.log('BUNNY DEBUG: Script block for ' + sanitizedId + ' is EXECUTING.');
        
        var videoId = '{{.VideoID}}';
        var player = null;
        var chapterWatcher = null;
        var iframeId = 'bunny-iframe-' + sanitizedId;
        var hasInitialized = false;

        function handleUpdateVideo(event) {
	        if( VERBOSE )
            console.log('BUNNY DEBUG: handleUpdateVideo CALLED for videoId: ' + event.detail.videoId);
          if (event.detail && event.detail.videoId === videoId && player && typeof player.setCurrentTime === 'function') {
            var container = document.getElementById('bunny-container-' + sanitizedId); // Re-acquire container
	          if( VERBOSE )
              console.log('BUNNY DEBUG: Seeking player ' + sanitizedId + ' to ' + event.detail.time);
            var timeString = String(event.detail.time).replace('s', '');
            var timeInSeconds = parseInt(timeString, 10);
            if (!isNaN(timeInSeconds)) {
              player.setCurrentTime(timeInSeconds);
              if (typeof player.play === 'function') {
                  player.play();
              }
              if (container) {
                container.scrollIntoView({ behavior: 'smooth', block: 'center' });
              }
            }
          }
        }

        function cleanupBunnyPlayer() {
	        if( VERBOSE )
            console.log('BUNNY DEBUG: cleanupBunnyPlayer CALLED for ' + sanitizedId);

          if (chapterWatcher) {
	          if( VERBOSE )
              console.log('BUNNY DEBUG: Clearing chapterWatcher interval ' + sanitizedId);
            clearInterval(chapterWatcher);
            chapterWatcher = null;
          }
          
	        if( VERBOSE )
            console.log('BUNNY DEBUG: REMOVING document event listener for update-video');
          document.removeEventListener('update-video', handleUpdateVideo);

          if (player) {
	          if( VERBOSE )
              console.log('BUNNY DEBUG: Pausing player ' + sanitizedId);
            if (typeof player.pause === 'function') {
                player.pause();
            }
            player = null; 
          }

          var iframe = document.getElementById(iframeId);
          if (iframe) {
	          if( VERBOSE )
              console.log('BUNNY DEBUG: Removing iframe ' + iframeId);
            iframe.src = 'about:blank';
            iframe.remove();
          }
          
	        if( VERBOSE )
            console.log('BUNNY DEBUG: hasInitialized SET to FALSE for ' + sanitizedId);
          hasInitialized = false;
        }

        function initBunnyPlayer() {
	        if( VERBOSE )
            console.log('BUNNY DEBUG: initBunnyPlayer CALLED for ' + sanitizedId);

          if (hasInitialized) {
	          if( VERBOSE )
              console.warn('BUNNY DEBUG: Init ' + sanitizedId + ' called but hasInitialized is TRUE. Aborting to prevent race condition.');
            return;
          }

          // The container MUST be re-acquired inside init(), because this function
          // is called on astro:page-load when the *old* container is gone.
          var container = document.getElementById('bunny-container-' + sanitizedId);
          if (!container) {
	          if( VERBOSE )
              console.error('BUNNY DEBUG: initBunnyPlayer FAILED to find container ' + sanitizedId + '. Aborting.');
            return;
          }

          hasInitialized = true;
	        if( VERBOSE )
            console.log('BUNNY DEBUG: hasInitialized SET to TRUE for ' + sanitizedId);

          var title = container.dataset.title;
          var chaptersJson = container.dataset.chapters;
          var loader = document.getElementById('bunny-loader-' + sanitizedId);
          var error = document.getElementById('bunny-error-' + sanitizedId);
          var playerContainer = container.querySelector('.aspect-video');

          if (!playerContainer) {
	          if( VERBOSE )
              console.error('BUNNY DEBUG: playerContainer not found for ' + sanitizedId);
            return;
          }
          
          var existingIframe = document.getElementById(iframeId);
          if (existingIframe) {
	          if( VERBOSE )
              console.warn('BUNNY DEBUG: Found STALE iframe for ' + sanitizedId + '. Removing it.');
            existingIframe.remove();
          }

          var chapters = [];
          var activeChapter = null;
          var hasChapters = false;

          if (chaptersJson) {
              try {
                  chapters = JSON.parse(chaptersJson);
              } catch(e) {
	                if( VERBOSE )
                    console.error("BUNNY DEBUG: Failed to parse bunny chapters:", e);
                  chapters = [];
              }
          }
          hasChapters = chapters.length > 0;

          var urlParams = new URLSearchParams(window.location.search);
          var startTime = urlParams.get('t') || urlParams.get('bunny_start');
          var timeInSeconds = -1;

          if (startTime) {
            var timeString = startTime.replace('s', '');
            var parsedTime = parseInt(timeString, 10);
            if (!isNaN(parsedTime)) {
              timeInSeconds = parsedTime;
            }
          }

	        if( VERBOSE )
            console.log('BUNNY DEBUG: Creating new iframe ' + iframeId);
          var iframe = document.createElement('iframe');
          var baseURL = 'https://iframe.mediadelivery.net/embed/' + videoId;
          
          var params = new URLSearchParams({
            autoplay: timeInSeconds >= 0 ? '1' : '0',
            preload: 'true',
            responsive: 'true',
            muted: 'false',
          });

          iframe.id = iframeId;
          iframe.src = baseURL + '?' + params.toString();
          iframe.className = 'w-full h-full absolute inset-0';
          iframe.title = title;
          iframe.setAttribute('allow', 'autoplay; fullscreen');
          iframe.setAttribute('loading', 'lazy');

          function formatTime(seconds) {
              var mins = Math.floor(seconds / 60);
              var secs = Math.floor(seconds % 60);
              return mins + ':' + secs.toString().padStart(2, '0');
          }

          function renderChaptersList() {
	            if( VERBOSE )
                console.log('BUNNY DEBUG: renderChaptersList ' + sanitizedId);
              var listEl = document.getElementById('bunny-chapters-list-' + sanitizedId);
              if (!listEl) return;
              listEl.innerHTML = '';
              chapters.forEach(function(chapter, index) {
                  var li = document.createElement('li');
                  li.className = 'p-3 hover:bg-gray-50 cursor-pointer';
                  li.dataset.startTime = chapter.startTime;
                  
                  var content = document.createElement('div');
                  content.className = 'flex justify-between items-center';
                  
                  var titleSpan = document.createElement('span');
                  titleSpan.className = 'flex items-center text-sm';
                  titleSpan.innerHTML = '<span class="w-6 h-6 flex items-center justify-center bg-gray-200 rounded-full text-xs mr-2">' + (index + 1) + '</span> ' + chapter.title;
                  
                  var timeLabel = document.createElement('span');
                  timeLabel.className = 'text-sm text-gray-500 ml-auto';
                  timeLabel.textContent = formatTime(chapter.startTime);

                  content.appendChild(titleSpan);
                  content.appendChild(timeLabel);
                  li.appendChild(content);

                  li.addEventListener('click', function() {
	                    if( VERBOSE )
                        console.log('BUNNY DEBUG: Chapter click ' + chapter.startTime);
                      if (player && typeof player.setCurrentTime === 'function') {
                          player.setCurrentTime(chapter.startTime);
                          if(typeof player.play === 'function') player.play();
                      }
                  });
                  listEl.appendChild(li);
              });
          }
          
          function navigateToLinkedPane(paneId) {
              if (!paneId) return;
	            if( VERBOSE )
                console.log('BUNNY DEBUG: NavigateToLinkedPane ' + paneId);
              var paneElement = document.getElementById('pane-' + paneId);
              if (!paneElement) {
	                if( VERBOSE )
                    console.warn('BUNNY DEBUG: Linked pane with id ' + paneId + ' not found.');
                  return;
              }
              if (player && typeof player.pause === 'function') {
                  player.pause();
              }
              var headerOffset = 60;
              var elementPosition = paneElement.getBoundingClientRect().top;
              var offsetPosition = elementPosition + window.pageYOffset - headerOffset;
              window.scrollTo({ top: offsetPosition, behavior: 'smooth' });
              paneElement.style.transition = 'box-shadow 0.5s ease-out';
              paneElement.style.boxShadow = '0 0 0 4px rgba(59, 130, 246, 0.5)';
              setTimeout(function() { paneElement.style.boxShadow = ''; }, 2000);
          }

          function updateChapterUI(chapter) {
              var overlay = document.getElementById('bunny-chapter-overlay-' + sanitizedId);
              var overlayTitle = document.getElementById('bunny-chapter-title-' + sanitizedId);
              var overlayLink = document.getElementById('bunny-chapter-link-' + sanitizedId);
              var listEl = document.getElementById('bunny-chapters-list-' + sanitizedId);

              if (chapter) {
                  if (overlay && overlayTitle) {
                      overlayTitle.textContent = chapter.title;
                      overlay.style.display = 'block';
                  }
                  if (overlayLink) {
                      overlayLink.style.display = chapter.linkedPaneId ? 'inline' : 'none';
                  }
                  if(listEl) {
                      listEl.querySelectorAll('li').forEach(function(item) { item.classList.remove('bg-blue-50'); });
                      var activeItem = listEl.querySelector('li[data-start-time="' + chapter.startTime + '"]');
                      if (activeItem) { activeItem.classList.add('bg-blue-50'); }
                  }
              } else {
                  if (overlay) { overlay.style.display = 'none'; }
                  if(listEl) {
                     listEl.querySelectorAll('li').forEach(function(item) { item.classList.remove('bg-blue-50'); });
                  }
              }
          }

          function checkCurrentTime(currentTime) {
              var chapter = chapters.find(function(c) { return currentTime >= c.startTime && currentTime < c.endTime; }) || null;
              if ((chapter && !activeChapter) || (chapter && activeChapter && chapter.startTime !== activeChapter.startTime)) {
                  activeChapter = chapter;
                  updateChapterUI(activeChapter);
              } else if (!chapter && activeChapter) {
                  activeChapter = null;
                  updateChapterUI(null);
              }
          }

          function startChapterTracking() {
	            if( VERBOSE )
                console.log('BUNNY DEBUG: startChapterTracking ' + sanitizedId);
              if (chapterWatcher) clearInterval(chapterWatcher);
              chapterWatcher = setInterval(function() {
                  if (player && typeof player.getCurrentTime === 'function') {
                      player.getCurrentTime(checkCurrentTime);
                  }
              }, 1000);
          }


          iframe.onload = function() {
	          if( VERBOSE )
              console.log('BUNNY DEBUG: iframe.onload FIRED for ' + iframeId);
            if (!window.playerjs) {
	            if( VERBOSE )
                console.error('BUNNY DEBUG: window.playerjs not found!');
              if (loader) loader.style.display = 'none';
              if (error) error.style.display = 'flex';
              return;
            }

            try {
	            if( VERBOSE )
                console.log('BUNNY DEBUG: Creating new window.playerjs.Player ' + iframeId);
              player = new window.playerjs.Player(iframeId);
              
              player.on('ready', function() {
	              if( VERBOSE )
                  console.log('BUNNY DEBUG: player.on(ready) FIRED for ' + iframeId);
                if (loader) loader.style.display = 'none';
                if (timeInSeconds >= 0 && player && typeof player.setCurrentTime === 'function') {
	                if( VERBOSE )
                    console.log('BUNNY DEBUG: Setting start time to ' + timeInSeconds);
                  player.setCurrentTime(timeInSeconds);
                  if (typeof player.play === 'function') {
                    player.play();
                  }
                  if (container) {
                    container.scrollIntoView({ behavior: 'smooth', block: 'center' });
                  }
                }
                if (hasChapters) {
                    startChapterTracking();
                }
              });

              player.on('error', function() {
	              if( VERBOSE )
                  console.error('BUNNY DEBUG: player.on(error) FIRED for ' + iframeId);
                if (loader) loader.style.display = 'none';
                if (error) error.style.display = 'flex';
              });
            } catch (e) {
	            if( VERBOSE )
                console.error('BUNNY DEBUG: Error initializing player.js: ', e);
              if (loader) loader.style.display = 'none';
              if (error) error.style.display = 'flex';
            }
          };
          
          if (playerContainer) {
	            if( VERBOSE )
                console.log('BUNNY DEBUG: Appending iframe ' + iframeId + ' to container.');
              playerContainer.appendChild(iframe);
          }

          if (hasChapters) {
              renderChaptersList();
              var header = document.getElementById('bunny-chapters-header-' + sanitizedId);
              var content = document.getElementById('bunny-chapters-content-' + sanitizedId);
              var overlayLink = document.getElementById('bunny-chapter-link-' + sanitizedId);

              if(header && content) {
                  header.addEventListener('click', function() {
                      var isHidden = content.classList.contains('hidden');
                      content.classList.toggle('hidden');
                      header.querySelector('.toggle-text').textContent = isHidden ? 'Hide' : 'Show';
                  });
              }
              if (overlayLink) {
                  overlayLink.addEventListener('click', function(e) {
                      e.preventDefault();
                      if (activeChapter && activeChapter.linkedPaneId) {
                          navigateToLinkedPane(activeChapter.linkedPaneId);
                      }
                  });
              }
          }
          
	        if( VERBOSE )
            console.log('BUNNY DEBUG: ADDING document event listener for update-video');
          document.addEventListener('update-video', handleUpdateVideo);
        }

	      if( VERBOSE )
          console.log('BUNNY DEBUG: ADDING Astro lifecycle listeners for ' + sanitizedId);
        document.addEventListener('astro:page-load', initBunnyPlayer);
        document.addEventListener('astro:before-swap', cleanupBunnyPlayer);

	      if( VERBOSE )
          console.log('BUNNY DEBUG: Calling IMMEDIATE initBunnyPlayer for ' + sanitizedId + ' (first load)');
        initBunnyPlayer();
      })();
    </script>
</div>{{end}}`))

type bunnyWidgetData struct {
	ClassNames       string
	Title            string
	VideoID          string
	SanitizedVideoID string
	ChaptersJSON     string
}

// RenderBunny generates the HTML and JavaScript required to embed a Bunny.net video player with the specified configuration.
func RenderBunny(classNames string, hook *rendering.CodeHook) string {
	if hook == nil || hook.Value1 == nil || hook.Value2 == nil || *hook.Value1 == "" || *hook.Value2 == "" {
		return `<div class="w-full aspect-video bg-gray-100 flex items-center justify-center text-center p-4"><div><p class="text-gray-500 mb-2">Bunny video is missing required Video ID or Title.</p></div></div>`
	}
	videoID := *hook.Value1
	title := *hook.Value2
	chaptersJSON := ""
	if hook.Value3 != "" {
		chaptersJSON = hook.Value3
	}

	sanitizedVideoID := strings.ReplaceAll(videoID, "/", "-")

	data := bunnyWidgetData{
		ClassNames:       classNames,
		Title:            title,
		VideoID:          videoID,
		SanitizedVideoID: sanitizedVideoID,
		ChaptersJSON:     chaptersJSON,
	}

	var buf bytes.Buffer
	err := bunnyWidgetTmpl.ExecuteTemplate(&buf, "bunnyWidget", data)
	if err != nil {
		log.Printf("ERROR: Failed to execute bunny widget template: %v", err)
		return ``
	}

	return buf.String()
}
