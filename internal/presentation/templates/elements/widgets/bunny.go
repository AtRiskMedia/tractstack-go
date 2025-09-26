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
        class="relative w-full aspect-video {{.ClassNames}}"
        data-video-id="{{.VideoID}}"
        data-title="{{.Title}}"
    >
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
    
    <script>
      (function() {
        const container = document.getElementById('bunny-container-{{.SanitizedVideoID}}');
        if (!container) return;

        const videoId = container.dataset.videoId;
        const title = container.dataset.title;
        const sanitizedId = '{{.SanitizedVideoID}}';
        const iframeId = 'bunny-iframe-' + sanitizedId;
        const loader = document.getElementById('bunny-loader-' + sanitizedId);
        const error = document.getElementById('bunny-error-' + sanitizedId);

        let player = null;

        const urlParams = new URLSearchParams(window.location.search);
        const startTime = urlParams.get('t') || urlParams.get('bunny_start');
        let timeInSeconds = -1;

        if (startTime) {
          const timeString = startTime.replace('s', '');
          const parsedTime = parseInt(timeString, 10);
          if (!isNaN(parsedTime)) {
            timeInSeconds = parsedTime;
          }
        }

        const iframe = document.createElement('iframe');
        const baseURL = 'https://iframe.mediadelivery.net/embed/' + videoId;
        
        const params = new URLSearchParams({
          autoplay: timeInSeconds >= 0 ? '1' : '0',
          preload: 'true',
          responsive: 'true',
          muted: 'false',
        });

        iframe.id = iframeId;
        iframe.src = baseURL + '?' + params.toString();
        iframe.className = 'w-full h-full';
        iframe.title = title;
        iframe.setAttribute('allow', 'autoplay; fullscreen');
        iframe.setAttribute('loading', 'lazy');

        iframe.onload = function() {
          if (!window.playerjs) {
            if (loader) loader.style.display = 'none';
            if (error) error.style.display = 'flex';
            return;
          }

          try {
            player = new window.playerjs.Player(iframeId);
            player.on('ready', function() {
              if (loader) loader.style.display = 'none';
              if (timeInSeconds >= 0 && player && typeof player.setCurrentTime === 'function') {
                player.setCurrentTime(timeInSeconds);
                if (typeof player.play === 'function') {
                  player.play();
                }
                container.scrollIntoView({ behavior: 'smooth', block: 'center' });
              }
            });
            player.on('error', function() {
              if (loader) loader.style.display = 'none';
              if (error) error.style.display = 'flex';
            });
          } catch (e) {
            console.error('Error initializing player.js:', e);
            if (loader) loader.style.display = 'none';
            if (error) error.style.display = 'flex';
          }
        };
        
        container.appendChild(iframe);

        document.addEventListener('update-video', function(event) {
          if (event.detail && event.detail.videoId === videoId && player && typeof player.setCurrentTime === 'function') {
            const timeString = String(event.detail.time).replace('s', '');
            const timeInSeconds = parseInt(timeString, 10);
            if (!isNaN(timeInSeconds)) {
              player.setCurrentTime(timeInSeconds);
              if (typeof player.play === 'function') {
                  player.play();
              }
            }
          }
        });
      })();
    </script>
</div>{{end}}`))

type bunnyWidgetData struct {
	ClassNames       string
	Title            string
	VideoID          string
	SanitizedVideoID string
}

func RenderBunny(classNames string, hook *rendering.CodeHook) string {
	if hook == nil || hook.Value1 == nil || hook.Value2 == nil || *hook.Value1 == "" || *hook.Value2 == "" {
		return `<div class="w-full aspect-video bg-gray-100 flex items-center justify-center text-center p-4"><div><p class="text-gray-500 mb-2">Bunny video is missing required Video ID or Title.</p></div></div>`
	}
	videoID := *hook.Value1
	title := *hook.Value2

	sanitizedVideoID := strings.ReplaceAll(videoID, "/", "-")

	data := bunnyWidgetData{
		ClassNames:       classNames,
		Title:            title,
		VideoID:          videoID,
		SanitizedVideoID: sanitizedVideoID,
	}

	var buf bytes.Buffer
	err := bunnyWidgetTmpl.ExecuteTemplate(&buf, "bunnyWidget", data)
	if err != nil {
		log.Printf("ERROR: Failed to execute bunny widget template: %v", err)
		return `<!-- template error -->`
	}

	return buf.String()
}
