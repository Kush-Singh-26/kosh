import re

with open('builder/services/post_service.go', 'r', encoding='utf-8') as f:
    content = f.read()

# Replace renderQueue initialization
old_init = """	// renderQueue is built via append to avoid sparse slots when posts are skipped
    renderQueue := make([]RenderContext, 0, len(files))"""
new_init = """	// renderQueue is built lock-free via atomic assignment to avoid mutex contention
	renderQueue := make([]RenderContext, len(files))
	var renderQueueIdx int32 = -1"""

# Also handle potential tab/space differences
content = re.sub(
    r'// renderQueue is built via append to avoid sparse slots when posts are skipped\s+renderQueue := make\(\[\]RenderContext, 0, len\(files\)\)',
    new_init,
    content
)

# Replace append with atomic assignment
# Look for the exact append logic
old_append = r"""renderQueue = append\(renderQueue, RenderContext\{\s+DestPath: destPath,\s+Version:\s+version,\s+Data: models\.PageData\{\s+Title: post\.Title, Description: post\.Description, Content: contentHTML,\s+Meta: metaData, BaseURL: s\.cfg\.BaseURL, BuildVersion: s\.cfg\.BuildVersion,\s+TabTitle: post\.Title \+ " \| " \+ s\.cfg\.Title, Permalink: post\.Link, Image: imagePath,\s+TOC: toc, Config: s\.cfg,\s+CurrentVersion: version,\s+IsOutdated:\s+s\.isOutdatedVersion\(version\),\s+Versions:\s+s\.cfg\.GetVersionsMetadata\(version, cleanHtmlRelPath\),\s+RelativePrefix: utils\.GetRelativePrefix\(htmlRelPath\),\s+ReadingTime:\s+post\.ReadingTime,\s+\},\s+\}\)"""

new_append = """idx := atomic.AddInt32(&renderQueueIdx, 1)
			renderQueue[idx] = RenderContext{
				DestPath: destPath,
				Version:  version,
				Data: models.PageData{
					Title: post.Title, Description: post.Description, Content: contentHTML,
					Meta: metaData, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
					TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: imagePath,
					TOC: toc, Config: s.cfg,
					CurrentVersion: version,
					IsOutdated:     s.isOutdatedVersion(version),
					Versions:       s.cfg.GetVersionsMetadata(version, cleanHtmlRelPath),
					RelativePrefix: utils.GetRelativePrefix(htmlRelPath),
					ReadingTime:    post.ReadingTime,
				},
			}"""
content = re.sub(old_append, new_append, content)

# Add slice compaction after cardPool phase starts and renderQueue needs slicing
old_compact = r"""// NOTE: cardPool is NOT stopped here — it continues generating social cards\s+// in parallel with the render phase below. Cards write to VFS/disk independently\s+// and the render phase only needs the imagePath URL string \(already computed\).\s+// cardPool.Stop\(\) is called after renderPool.Stop\(\) to overlap both phases."""

new_compact = """// NOTE: cardPool is NOT stopped here — it continues generating social cards
	// in parallel with the render phase below. Cards write to VFS/disk independently
	// and the render phase only needs the imagePath URL string (already computed).
	// cardPool.Stop() is called after renderPool.Stop() to overlap both phases.

	// Compact renderQueue to the exact number of elements rendered
	finalRenderCount := int(renderQueueIdx + 1)
	renderQueue = renderQueue[:finalRenderCount]"""
content = re.sub(old_compact, new_compact, content)

with open('builder/services/post_service.go', 'w', encoding='utf-8') as f:
    f.write(content)

