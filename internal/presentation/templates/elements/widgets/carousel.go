package templates

import (
	"bytes"
	"fmt"
	"html/template"
)

var wordCarouselTmpl = template.Must(template.New("wordCarousel").Parse(
	`<script>
(function() {
  var nodeId = "{{.NodeID}}";
  var speed = {{.Speed}};
  var intervalId = null;
  
  function initWordCarousel() {
    var el = document.getElementById(nodeId);
    if (!el) return;
    
    var wordsRaw = el.getAttribute('data-word-carousel-words');
    if (!wordsRaw) return;
    
    var words = [];
    try {
      words = JSON.parse(wordsRaw);
    } catch (e) {
      console.error('WordCarousel: Failed to parse words', e);
      return;
    }
    
    if (words.length < 2) return;
    
    var currentIndex = 0;
    // Start matching what is currently visible if possible, otherwise 0
    var currentText = el.innerText;
    var foundIndex = words.indexOf(currentText);
    if (foundIndex > -1) currentIndex = foundIndex;

    if (intervalId) clearInterval(intervalId);
    
    intervalId = setInterval(function() {
      currentIndex = (currentIndex + 1) % words.length;
      el.innerText = words[currentIndex];
    }, speed * 1000);
  }

  function cleanupWordCarousel() {
    if (intervalId) {
      clearInterval(intervalId);
      intervalId = null;
    }
    document.removeEventListener('astro:page-load', initWordCarousel);
    document.removeEventListener('astro:before-swap', cleanupWordCarousel);
  }

  document.addEventListener('astro:page-load', initWordCarousel);
  document.addEventListener('astro:before-swap', cleanupWordCarousel);
  
  // Immediate init for initial load
  initWordCarousel();
})();
</script>`))

type wordCarouselData struct {
	NodeID string
	Speed  float64
}

func RenderWordCarousel(nodeID string, speed float64) string {
	data := wordCarouselData{
		NodeID: nodeID,
		Speed:  speed,
	}

	var buf bytes.Buffer
	if err := wordCarouselTmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("ERROR: Failed to execute wordCarousel template: %v", err)
	}
	return buf.String()
}
