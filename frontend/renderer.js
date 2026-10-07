const state = {
  directories: [],
  scanResult: null,
  isScanning: false,
  isCancellingScan: false,
  isDeleting: false,
  isCancellingDelete: false,
  selectedFiles: new Set(),
  collapsedGroups: new Set(),
  filters: {
    fileType: 'all',
    fileSize: 'all',
  },
  scanProgress: {
    percent: 0,
    stage: '未扫描',
    detail: '',
  },
  deleteProgress: {
    percent: 0,
    current: 0,
    total: 0,
    stage: '未删除',
    detail: '',
  },
  thumbnailVersion: 0,
  resultView: {
    revision: 0,
    filteredGroupsRevision: -1,
    filteredGroups: [],
    flatItemsRevision: -1,
    flatItems: [],
    heightCache: new Map(),
    renderFrameId: 0,
  },
};

const elements = {
  appTitle: document.getElementById('appTitle'),
  scanHelpButton: document.getElementById('scanHelpButton'),
  selectDirectoriesButton: document.getElementById('selectDirectoriesButton'),
  scanButton: document.getElementById('scanButton'),
  cancelScanButton: document.getElementById('cancelScanButton'),
  clearButton: document.getElementById('clearButton'),
  selectAllButton: document.getElementById('selectAllButton'),
  invertSelectButton: document.getElementById('invertSelectButton'),
  autoSelectButton: document.getElementById('autoSelectButton'),
  clearThumbnailCacheButton: document.getElementById('clearThumbnailCacheButton'),
  deleteButton: document.getElementById('deleteButton'),
  fileTypeFilter: document.getElementById('fileTypeFilter'),
  fileSizeFilter: document.getElementById('fileSizeFilter'),
  directoryList: document.getElementById('directoryList'),
  directoryCount: document.getElementById('directoryCount'),
  statusText: document.getElementById('statusText'),
  scanState: document.getElementById('scanState'),
  scanProgressSection: document.getElementById('scanProgressSection'),
  scanProgressStage: document.getElementById('scanProgressStage'),
  scanProgressPercent: document.getElementById('scanProgressPercent'),
  scanProgressBar: document.getElementById('scanProgressBar'),
  scanProgressDetail: document.getElementById('scanProgressDetail'),
  deleteProgressModal: document.getElementById('deleteProgressModal'),
  cancelDeleteButton: document.getElementById('cancelDeleteButton'),
  deleteProgressStage: document.getElementById('deleteProgressStage'),
  deleteProgressCount: document.getElementById('deleteProgressCount'),
  deleteProgressPercent: document.getElementById('deleteProgressPercent'),
  deleteProgressBar: document.getElementById('deleteProgressBar'),
  deleteProgressDetail: document.getElementById('deleteProgressDetail'),
  totalFilesValue: document.getElementById('totalFilesValue'),
  candidateFilesValue: document.getElementById('candidateFilesValue'),
  duplicateGroupsValue: document.getElementById('duplicateGroupsValue'),
  wastedSizeValue: document.getElementById('wastedSizeValue'),
  resultCount: document.getElementById('resultCount'),
  selectedCount: document.getElementById('selectedCount'),
  results: document.getElementById('results'),
  resultToolbar: document.querySelector('.result-toolbar'),
  skippedCount: document.getElementById('skippedCount'),
  skippedList: document.getElementById('skippedList'),
  content: document.querySelector('.content'),
  scrollToTopButton: document.getElementById('scrollToTopButton'),
};

const RESULT_VIRTUALIZATION_THRESHOLD = 180;
const RESULT_VIRTUALIZATION_OVERSCAN = 720;
const RESULT_ITEM_ESTIMATED_HEIGHTS = {
  group: 44,
  file: 86,
  tip: 30,
};

elements.selectDirectoriesButton.addEventListener('click', handleSelectDirectories);
elements.scanButton.addEventListener('click', handleScan);
elements.cancelScanButton.addEventListener('click', handleCancelScan);
elements.clearButton.addEventListener('click', handleClearDirectories);
elements.selectAllButton.addEventListener('click', handleSelectAllFiles);
elements.invertSelectButton.addEventListener('click', handleInvertSelectedFiles);
elements.autoSelectButton.addEventListener('click', handleAutoSelectDuplicates);
elements.clearThumbnailCacheButton.addEventListener('click', handleClearThumbnailCache);
elements.deleteButton.addEventListener('click', handleDeleteSelectedFiles);
elements.cancelDeleteButton.addEventListener('click', handleCancelDelete);
elements.fileTypeFilter.addEventListener('change', handleFilterChange);
elements.fileSizeFilter.addEventListener('change', handleFilterChange);
elements.results.addEventListener('change', handleResultsChange);
elements.results.addEventListener('click', handleResultsClick);
if (elements.scrollToTopButton) {
  elements.scrollToTopButton.addEventListener('click', handleScrollToTop);
}
if (elements.content) {
  elements.content.addEventListener('scroll', handleResultsViewportChange, { passive: true });
}
window.addEventListener('resize', handleResultsViewportChange);

if (
  window.duplicateFinderAPI &&
  typeof window.duplicateFinderAPI.onScanProgress === 'function'
) {
  window.duplicateFinderAPI.onScanProgress((progress) => {
    state.scanProgress = {
      percent: progress.percent || 0,
      stage: progress.stage || '处理中',
      detail: progress.detail || '',
    };
    renderScanProgress();
  });
}

if (
  window.duplicateFinderAPI &&
  typeof window.duplicateFinderAPI.onDeleteProgress === 'function'
) {
  window.duplicateFinderAPI.onDeleteProgress((progress) => {
    state.deleteProgress = {
      percent: progress.percent || 0,
      current: progress.current || 0,
      total: progress.total || 0,
      stage: progress.stage || '处理中',
      detail: progress.detail || '',
    };
    renderDeleteProgressModal();
  });
}

initializeAppTitle();
render();
updateResultToolbarShadow();
updateScrollToTopButton();

async function initializeAppTitle() {
  try {
    if (
      !window.duplicateFinderAPI ||
      typeof window.duplicateFinderAPI.getAppVersion !== 'function'
    ) {
      return;
    }

    const version = await window.duplicateFinderAPI.getAppVersion();
    if (!version) {
      return;
    }

    const title = `重复文件查找器 v${version}`;
    document.title = title;
    if (elements.appTitle) {
      elements.appTitle.textContent = title;
    }
  } catch {
    // Keep the fallback title when version retrieval is unavailable.
  }
}

async function handleSelectDirectories() {
  try {
    const selectedDirectories = await window.duplicateFinderAPI.selectDirectories();
    await waitForNextFrame();
    addDirectories(selectedDirectories);
  } catch (error) {
    const message = (error && error.message) || '选择目录失败，请重试。';
    state.scanResult = {
      ...(state.scanResult || {}),
      error: message,
    };
    render();
    window.alert(message);
  }
}

async function handleScan() {
  if (state.directories.length === 0 || state.isScanning) {
    return;
  }

  state.isScanning = true;
  state.isCancellingScan = false;
  state.scanResult = null;
  state.selectedFiles = new Set();
  state.collapsedGroups = new Set();
  invalidateResultCaches({ clearHeights: true });
  state.scanProgress = {
    percent: 0,
    stage: '准备扫描目录',
    detail: '',
  };
  render();

  try {
    state.scanResult = await window.duplicateFinderAPI.scanDirectories(state.directories);
    invalidateResultCaches({ clearHeights: true });
  } catch (error) {
    state.scanResult = {
      error: error.message || '扫描失败，请稍后再试。',
    };
    invalidateResultCaches({ clearHeights: true });
  } finally {
    state.isScanning = false;
    state.isCancellingScan = false;
    render();
  }
}

async function handleCancelScan() {
  if (!state.isScanning || state.isCancellingScan) {
    return;
  }

  state.isCancellingScan = true;
  state.scanProgress = {
    ...state.scanProgress,
    stage: '正在中止扫描',
    detail: state.scanProgress.detail || '等待当前文件处理结束',
  };
  render();

  try {
    await window.duplicateFinderAPI.cancelScan();
  } catch (error) {
    state.isCancellingScan = false;
    state.scanResult = {
      ...(state.scanResult || {}),
      error: (error && error.message) || '中止扫描失败，请重试。',
    };
    render();
  }
}

async function handleClearThumbnailCache() {
  if (
    state.isScanning ||
    state.isDeleting ||
    !window.duplicateFinderAPI ||
    typeof window.duplicateFinderAPI.clearThumbnailCache !== 'function'
  ) {
    return;
  }

  elements.clearThumbnailCacheButton.disabled = true;

  try {
    const message = await window.duplicateFinderAPI.clearThumbnailCache();
    state.thumbnailVersion += 1;
    render();
    if (message) {
      window.alert(message);
    }
  } catch (error) {
    window.alert((error && error.message) || '清除缩略图缓存失败，请重试。');
  } finally {
    elements.clearThumbnailCacheButton.disabled = state.isScanning || state.isDeleting;
  }
}

function handleClearDirectories() {
  state.directories = [];
  state.scanResult = null;
  state.selectedFiles = new Set();
  state.collapsedGroups = new Set();
  invalidateResultCaches({ clearHeights: true });
  render();
}

function handleRemoveDirectory(directory) {
  state.directories = state.directories.filter((item) => item !== directory);
  state.scanResult = null;
  state.selectedFiles = new Set();
  state.collapsedGroups = new Set();
  invalidateResultCaches({ clearHeights: true });
  render();
}

function addDirectories(directories) {
  if (!directories || directories.length === 0) {
    return;
  }

  const directorySet = new Set(state.directories);
  directories.forEach((directory) => {
    if (directory && !directorySet.has(directory)) {
      directorySet.add(directory);
    }
  });

  state.directories = [...directorySet];
  state.scanResult = null;
  state.selectedFiles = new Set();
  state.collapsedGroups = new Set();
  invalidateResultCaches({ clearHeights: true });
  state.scanProgress = {
    percent: 0,
    stage: '未扫描',
    detail: '',
  };
  render();
}

function handleAutoSelectDuplicates() {
  const groups = getFilteredGroups();
  if (groups.length === 0) {
    return;
  }

  const nextSelectedFiles = new Set();

  for (const group of groups) {
    group.files.slice(1).forEach((file) => {
      nextSelectedFiles.add(file.path);
    });
  }

  state.selectedFiles = nextSelectedFiles;
  render();
}

function handleSelectAllFiles() {
  const groups = getFilteredGroups();
  if (groups.length === 0) {
    return;
  }

  const nextSelectedFiles = new Set();

  for (const group of groups) {
    group.files.forEach((file) => {
      nextSelectedFiles.add(file.path);
    });
  }

  state.selectedFiles = nextSelectedFiles;
  render();
}

function handleInvertSelectedFiles() {
  const groups = getFilteredGroups();
  if (groups.length === 0) {
    return;
  }

  const currentSelectedFiles = state.selectedFiles;
  const nextSelectedFiles = new Set();

  for (const group of groups) {
    group.files.forEach((file) => {
      if (!currentSelectedFiles.has(file.path)) {
        nextSelectedFiles.add(file.path);
      }
    });
  }

  state.selectedFiles = nextSelectedFiles;
  render();
}

async function handleDeleteSelectedFiles() {
  const selectedPaths = [...state.selectedFiles];
  if (selectedPaths.length === 0 || state.isScanning || state.isDeleting) {
    return;
  }

  state.isDeleting = true;
  state.isCancellingDelete = false;
  state.deleteProgress = {
    percent: 0,
    current: 0,
    total: selectedPaths.length,
    stage: '准备删除文件',
    detail: `共 ${selectedPaths.length} 个文件`,
  };
  render();

  try {
    const result = await window.duplicateFinderAPI.deleteFiles(selectedPaths);
    const currentGroups = state.scanResult && state.scanResult.groups ? state.scanResult.groups : [];
    const currentSummary = state.scanResult && state.scanResult.summary ? state.scanResult.summary : null;
    const currentSkipped = state.scanResult && state.scanResult.skipped ? state.scanResult.skipped : [];
    const groups = filterDeletedFilesFromGroups(currentGroups, result.deleted || []);
    const failedItems = result.failed || [];
    const deletedCount = (result.deleted || []).length;
    const failedCount = failedItems.length;
    const isCancelled = Boolean(result.cancelled);
    let message = '';

    if (isCancelled && failedCount > 0) {
      message = `已删除 ${deletedCount} 个文件，${failedCount} 个删除失败，剩余操作已取消。`;
    } else if (isCancelled && deletedCount > 0) {
      message = `已删除 ${deletedCount} 个文件，剩余操作已取消。`;
    } else if (isCancelled) {
      message = '已取消删除操作。';
    } else if (failedCount > 0) {
      message = `已删除 ${deletedCount} 个文件，${failedCount} 个删除失败。`;
    }

    if (message) {
      state.scanResult = {
        ...(state.scanResult || {}),
        error: message,
        groups,
        summary: buildSummaryFromGroups(currentSummary, groups, deletedCount),
        skipped: [
          ...currentSkipped,
          ...failedItems,
        ],
      };
      invalidateResultCaches({ clearHeights: true });
    } else {
      state.scanResult = {
        ...(state.scanResult || {}),
        groups,
        summary: buildSummaryFromGroups(currentSummary, groups, deletedCount),
      };
      invalidateResultCaches({ clearHeights: true });
    }

    state.selectedFiles = new Set();
  } catch (error) {
    state.scanResult = {
      ...(state.scanResult || {}),
      error: error.message || '删除失败，请稍后再试。',
    };
    invalidateResultCaches({ clearHeights: true });
  } finally {
    state.isDeleting = false;
    state.isCancellingDelete = false;
    render();
  }
}

async function handleCancelDelete() {
  if (!state.isDeleting || state.isCancellingDelete) {
    return;
  }

  state.isCancellingDelete = true;
  state.deleteProgress = {
    ...state.deleteProgress,
    stage: '正在中止删除',
    detail: state.deleteProgress.detail || '等待当前批次处理结束',
  };
  render();

  try {
    await window.duplicateFinderAPI.cancelDelete();
  } catch (error) {
    state.isCancellingDelete = false;
    state.scanResult = {
      ...(state.scanResult || {}),
      error: (error && error.message) || '取消删除失败，请重试。',
    };
    render();
  }
}

function handleFilterChange() {
  state.filters = {
    fileType: elements.fileTypeFilter.value,
    fileSize: elements.fileSizeFilter.value,
  };
  invalidateResultCaches();
  syncSelectedFilesToVisibleGroups();
  render();
}

function render() {
  renderDirectoryList();
  renderScanState();
  renderScanProgress();
  renderDeleteProgressModal();
  renderSummary();
  renderFileTypeFilterOptions();
  renderResults();
  renderSkipped();
  updateButtons();
  updateScrollToTopButton();
}

function renderDirectoryList() {
  elements.directoryCount.textContent = `${state.directories.length} 个`;

  if (state.directories.length === 0) {
    elements.directoryList.innerHTML = '<li class="empty-state">还没有选择目录</li>';
    return;
  }

  elements.directoryList.innerHTML = state.directories
    .map(
      (directory) => `
        <li class="directory-item">
          <span class="directory-path">${escapeHtml(directory)}</span>
          <button
            class="remove-button"
            data-directory="${escapeAttribute(directory)}"
            title="移除目录"
            aria-label="移除目录"
          >
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M7 7l10 10M17 7 7 17" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
            </svg>
          </button>
        </li>
      `,
    )
    .join('');

  elements.directoryList.querySelectorAll('.remove-button').forEach((button) => {
    button.addEventListener('click', () => {
      handleRemoveDirectory(button.dataset.directory);
    });
  });
}

function renderScanState() {
  if (state.isScanning) {
    elements.statusText.textContent = state.isCancellingScan
      ? '正在中止扫描，请稍候...'
      : '正在扫描文件并计算重复结果，请稍候...';
    elements.scanState.textContent = '处理中';
    elements.scanState.className = 'scan-state running';
    return;
  }

  if (state.isDeleting) {
    elements.statusText.textContent = state.isCancellingDelete
      ? '正在中止删除，请稍候...'
      : '正在删除已选文件，请稍候...';
    elements.scanState.textContent = '处理中';
    elements.scanState.className = 'scan-state running';
    return;
  }

  if (state.scanResult && state.scanResult.error) {
    elements.statusText.textContent = state.scanResult.error;
    elements.scanState.textContent = '提示';
    elements.scanState.className = 'scan-state error';
    return;
  }

  if (state.scanResult) {
    const { summary } = state.scanResult;
    elements.statusText.textContent = `已扫描 ${summary.totalFiles} 个文件，找到 ${summary.duplicateGroups} 组完全相同的文件。`;
    elements.scanState.textContent = '已完成';
    elements.scanState.className = 'scan-state done';
    return;
  }

  elements.statusText.textContent = '请选择目录后开始扫描。';
  elements.scanState.textContent = '未扫描';
  elements.scanState.className = 'scan-state idle';
}

function renderScanProgress() {
  if (!state.isScanning || state.scanResult) {
    elements.scanProgressSection.classList.add('is-hidden');
    elements.scanProgressBar.style.width = '0%';
    return;
  }

  elements.scanProgressSection.classList.remove('is-hidden');
  elements.scanProgressStage.textContent = state.scanProgress.stage || '处理中';
  elements.scanProgressPercent.textContent = `${Math.max(0, Math.min(100, state.scanProgress.percent || 0))}%`;
  elements.scanProgressBar.style.width = `${Math.max(0, Math.min(100, state.scanProgress.percent || 0))}%`;
  elements.scanProgressDetail.textContent = state.scanProgress.detail || '正在扫描，请稍候...';
}

function renderDeleteProgressModal() {
  if (!state.isDeleting) {
    elements.deleteProgressModal.classList.add('is-hidden');
    elements.deleteProgressModal.setAttribute('aria-hidden', 'true');
    elements.deleteProgressCount.textContent = '0/0';
    elements.deleteProgressBar.style.width = '0%';
    return;
  }

  const percent = Math.max(0, Math.min(100, state.deleteProgress.percent || 0));
  const current = Math.max(0, state.deleteProgress.current || 0);
  const total = Math.max(current, state.deleteProgress.total || 0);
  elements.deleteProgressModal.classList.remove('is-hidden');
  elements.deleteProgressModal.setAttribute('aria-hidden', 'false');
  elements.deleteProgressStage.textContent = state.deleteProgress.stage || '正在删除文件';
  elements.deleteProgressCount.textContent = `${current}/${total}`;
  elements.deleteProgressPercent.textContent = `${percent}%`;
  elements.deleteProgressBar.style.width = `${percent}%`;
  elements.deleteProgressDetail.textContent = state.deleteProgress.detail || '正在删除，请稍候...';
}

function renderSummary() {
  const summary = state.scanResult ? state.scanResult.summary : null;

  elements.totalFilesValue.textContent = summary ? formatNumber(summary.totalFiles) : '0';
  elements.candidateFilesValue.textContent = summary ? formatNumber(summary.candidateFiles) : '0';
  elements.duplicateGroupsValue.textContent = summary ? formatNumber(summary.duplicateGroups) : '0';
  elements.wastedSizeValue.textContent = summary ? formatBytes(summary.wastedSize) : '0 B';
}

function renderResults() {
  const groups = getFilteredGroups();
  const items = getFlatResultItems();
  elements.resultCount.textContent = `${groups.length} 组`;
  elements.selectedCount.textContent = `已选 ${state.selectedFiles.size} 个`;
  elements.fileSizeFilter.value = state.filters.fileSize;

  if (
    state.scanResult &&
    state.scanResult.error &&
    (((state.scanResult.groups && state.scanResult.groups.length) || 0) === 0) &&
    groups.length === 0
  ) {
    elements.results.innerHTML = '<div class="empty-state">处理失败，请调整后重试。</div>';
    return;
  }

  if (state.isScanning && !state.scanResult) {
    elements.results.innerHTML = '<div class="empty-state">扫描中，结果会在完成后显示。</div>';
    return;
  }

  if (items.length === 0) {
    elements.results.innerHTML = state.scanResult && state.scanResult.groups && state.scanResult.groups.length
      ? '<div class="empty-state">当前筛选条件下没有匹配结果。</div>'
      : '<div class="empty-state">当前没有找到完全相同的文件。</div>';
    return;
  }

  const virtualState = getVirtualizedResultState(items);
  elements.results.innerHTML = renderVirtualizedResultItems(virtualState);

  elements.results.querySelectorAll('.group-checkbox').forEach((checkbox) => {
    const group = groups.find((item) => item.id === checkbox.dataset.groupId);
    if (!group) {
      return;
    }

    checkbox.indeterminate = getGroupSelectionState(group).indeterminate;
  });

  measureVisibleResultItems();
}

function renderFileRow(file, isDuplicate, options = {}) {
  const checked = state.selectedFiles.has(file.path);
  const shouldEmphasizeDirectory = Boolean(options.shouldEmphasizeDirectory);
  const locationLabel = shouldEmphasizeDirectory ? '目录' : '位置';
  const locationValue = shouldEmphasizeDirectory ? file.directory : file.path;
  const locationClassName = shouldEmphasizeDirectory
    ? 'support-item support-item-path support-item-path-emphasis'
    : 'support-item support-item-path';

  return `
    <div
      class="result-row ${isDuplicate ? 'is-duplicate' : ''} ${checked ? 'is-selected' : ''}"
      data-path="${escapeAttribute(file.path)}"
    >
      <div class="checkbox-cell">
        <input
          class="row-checkbox"
          type="checkbox"
          data-path="${escapeAttribute(file.path)}"
          ${checked ? 'checked' : ''}
        />
      </div>
      <button
        class="preview-button"
        type="button"
        data-path="${escapeAttribute(file.path)}"
        title="预览文件"
        aria-label="预览文件"
      >
        ${renderFileThumbnail(file)}
      </button>
      <div class="file-cell">
        <span class="file-name">${escapeHtml(file.name)}</span>
        <div class="file-support">
          <span class="support-item">大小：${formatBytes(file.size)}</span>
          <span class="support-item">类型：${escapeHtml(file.extension)}</span>
          <span class="support-item">修改时间：${escapeHtml(formatDateTime(file.modifiedAt))}</span>
          <span class="${locationClassName}">${locationLabel}：${escapeHtml(locationValue)}</span>
        </div>
      </div>
      <div class="action-cell">
        <button
          class="icon-button reveal-button"
          type="button"
          data-path="${escapeAttribute(file.path)}"
          title="打开所在文件夹"
          aria-label="打开所在文件夹"
        >
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M4 6.75A2.75 2.75 0 0 1 6.75 4h3.57c.73 0 1.42.3 1.94.82l.92.93h4.07A2.75 2.75 0 0 1 20 8.5v8.75A2.75 2.75 0 0 1 17.25 20H6.75A2.75 2.75 0 0 1 4 17.25V6.75Z" stroke="currentColor" stroke-width="1.7"/>
            <path d="M9 12h6M12 9l3 3-3 3" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>
      </div>
    </div>
  `;
}

function renderFileThumbnail(file) {
  if (isImageFile(file.extension)) {
    return `
      <img
        class="thumbnail-image"
        src="${escapeAttribute(toThumbnailUrl(file.path, 100))}"
        alt="${escapeAttribute(file.name)}"
        loading="lazy"
      />
    `;
  }

  return `
    <span class="thumbnail-fallback" aria-hidden="true">
      <svg viewBox="0 0 24 24" fill="none">
        <path d="M7 3.75h7.38a2 2 0 0 1 1.41.59l2.87 2.87a2 2 0 0 1 .59 1.41v8.63A2.75 2.75 0 0 1 16.5 20H7.5A2.75 2.75 0 0 1 4.75 17.25v-10A3.5 3.5 0 0 1 8.25 3.75Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/>
        <path d="M14 3.75v4h4" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/>
      </svg>
    </span>
  `;
}

function renderSkipped() {
  const skippedItems = state.scanResult && state.scanResult.skipped ? state.scanResult.skipped : [];
  elements.skippedCount.textContent = `${skippedItems.length} 项`;

  if (skippedItems.length === 0) {
    elements.skippedList.innerHTML = '<div class="empty-state">目前没有跳过项。</div>';
    return;
  }

  elements.skippedList.innerHTML = skippedItems
    .map(
      (item) => `
        <div class="skipped-item">
          <div class="skipped-path">${escapeHtml(item.path)}</div>
          <div class="skipped-reason">${escapeHtml(item.reason)}</div>
        </div>
      `,
    )
    .join('');
}

function toggleFileSelection(filePath, checked) {
  const nextSelectedFiles = new Set(state.selectedFiles);

  if (checked) {
    nextSelectedFiles.add(filePath);
  } else {
    nextSelectedFiles.delete(filePath);
  }

  state.selectedFiles = nextSelectedFiles;
  render();
}

function handleResultsChange(event) {
  const groupCheckbox = event.target.closest('.group-checkbox');
  if (groupCheckbox) {
    toggleGroupSelection(groupCheckbox.dataset.groupId, groupCheckbox.checked);
    return;
  }

  const checkbox = event.target.closest('.row-checkbox');
  if (!checkbox) {
    return;
  }

  toggleFileSelection(checkbox.dataset.path, checkbox.checked);
}

async function handleResultsClick(event) {
  if (state.isScanning || state.isDeleting) {
    return;
  }

  const collapseButton = event.target.closest('.group-collapse-button');
  if (collapseButton) {
    event.stopPropagation();
    toggleGroupCollapsed(collapseButton.dataset.groupId);
    return;
  }

  const groupCheckbox = event.target.closest('.group-checkbox');
  if (groupCheckbox) {
    event.stopPropagation();
    return;
  }

  const previewButton = event.target.closest('.preview-button');
  if (previewButton) {
    event.stopPropagation();
    try {
      await window.duplicateFinderAPI.previewFile(previewButton.dataset.path);
    } catch (error) {
      window.alert((error && error.message) || '预览文件失败，请重试。');
    }
    return;
  }

  const revealButton = event.target.closest('.reveal-button');
  if (revealButton) {
    event.stopPropagation();
    try {
      await window.duplicateFinderAPI.revealFile(revealButton.dataset.path);
    } catch (error) {
      window.alert((error && error.message) || '打开所在文件夹失败，请重试。');
    }
    return;
  }

  const checkbox = event.target.closest('.row-checkbox');
  if (checkbox) {
    event.stopPropagation();
    return;
  }

  const row = event.target.closest('.result-row');
  if (!row) {
    return;
  }

  toggleFileSelection(row.dataset.path, !state.selectedFiles.has(row.dataset.path));
}

function toggleGroupCollapsed(groupId) {
  const nextCollapsedGroups = new Set(state.collapsedGroups);

  if (nextCollapsedGroups.has(groupId)) {
    nextCollapsedGroups.delete(groupId);
  } else {
    nextCollapsedGroups.add(groupId);
  }

  state.collapsedGroups = nextCollapsedGroups;
  invalidateResultCaches();
  render();
}

function toggleGroupSelection(groupId, checked) {
  const group = getFilteredGroups().find((item) => item.id === groupId);
  if (!group) {
    return;
  }

  const nextSelectedFiles = new Set(state.selectedFiles);

  group.files.forEach((file) => {
    if (checked) {
      nextSelectedFiles.add(file.path);
    } else {
      nextSelectedFiles.delete(file.path);
    }
  });

  state.selectedFiles = nextSelectedFiles;
  render();
}

function getGroupSelectionState(group) {
  const selectedCount = group.files.filter((file) => state.selectedFiles.has(file.path)).length;

  return {
    checked: selectedCount === group.files.length && group.files.length > 0,
    indeterminate: selectedCount > 0 && selectedCount < group.files.length,
  };
}

function hasMultipleFileDirectories(group) {
  if (!group || !group.files || group.files.length < 2) {
    return false;
  }

  const directories = new Set();
  group.files.forEach((file) => {
    const directory = (file && file.directory) || '';
    if (directory) {
      directories.add(directory);
    }
  });

  return directories.size > 1;
}

function updateButtons() {
  const isBusy = state.isScanning || state.isDeleting;
  const scanDisabled = state.directories.length === 0 || isBusy;
  const hasScanGroups = Boolean(state.scanResult && state.scanResult.groups && state.scanResult.groups.length);
  const hasVisibleGroups = getFilteredGroups().length > 0;

  elements.scanButton.disabled = scanDisabled;
  elements.cancelScanButton.disabled = !state.isScanning || state.isCancellingScan || state.isDeleting;
  elements.clearButton.disabled = state.directories.length === 0 || isBusy;
  elements.selectDirectoriesButton.disabled = isBusy;
  elements.selectAllButton.disabled = !hasVisibleGroups || isBusy;
  elements.invertSelectButton.disabled = !hasVisibleGroups || isBusy;
  elements.autoSelectButton.disabled = !hasVisibleGroups || isBusy;
  elements.clearThumbnailCacheButton.disabled = isBusy;
  elements.deleteButton.disabled = state.selectedFiles.size === 0 || isBusy;
  elements.fileTypeFilter.disabled = !hasScanGroups || isBusy;
  elements.fileSizeFilter.disabled = !hasScanGroups || isBusy;
  elements.cancelDeleteButton.disabled = !state.isDeleting || state.isCancellingDelete;
}

function renderFileTypeFilterOptions() {
  const options = getAvailableFileTypeOptions();

  if (state.filters.fileType !== 'all' && !options.some((option) => option.value === state.filters.fileType)) {
    state.filters = {
      ...state.filters,
      fileType: 'all',
    };
    invalidateResultCaches();
    syncSelectedFilesToVisibleGroups();
  }

  const nextMarkup = [
    '<option value="all">全部类型</option>',
    ...options.map(
      (option) => `<option value="${escapeAttribute(option.value)}">${escapeHtml(option.label)}</option>`,
    ),
  ].join('');

  if (elements.fileTypeFilter.innerHTML !== nextMarkup) {
    elements.fileTypeFilter.innerHTML = nextMarkup;
  }

  elements.fileTypeFilter.value = state.filters.fileType;
}

function filterDeletedFilesFromGroups(groups, deletedPaths) {
  const deletedSet = new Set(deletedPaths);

  return groups
    .map((group) => {
      const files = group.files.filter((file) => !deletedSet.has(file.path));

      if (files.length < 2) {
        return null;
      }

      return {
        ...group,
        files,
        fileCount: files.length,
        duplicateSize: group.size * (files.length - 1),
        directories: [...new Set(files.map((file) => file.rootDirectory))],
      };
    })
    .filter(Boolean);
}

function buildSummaryFromGroups(previousSummary, groups, deletedCount) {
  return {
    totalFiles: Math.max(((previousSummary && previousSummary.totalFiles) || 0) - deletedCount, 0),
    candidateFiles: Math.max(((previousSummary && previousSummary.candidateFiles) || 0) - deletedCount, 0),
    duplicateGroups: groups.length,
    duplicateFiles: groups.reduce((sum, group) => sum + group.fileCount, 0),
    wastedSize: groups.reduce((sum, group) => sum + group.duplicateSize, 0),
  };
}

function getFilteredGroups() {
  if (state.resultView.filteredGroupsRevision === state.resultView.revision) {
    return state.resultView.filteredGroups;
  }

  const groups = state.scanResult && state.scanResult.groups ? state.scanResult.groups : [];

  const filteredGroups = groups.filter((group) => {
    const matchesType = matchesFileTypeFilter(group);
    const matchesSize = matchesFileSizeFilter(group);
    return matchesType && matchesSize;
  });

  state.resultView.filteredGroups = filteredGroups;
  state.resultView.filteredGroupsRevision = state.resultView.revision;
  return filteredGroups;
}

function matchesFileTypeFilter(group) {
  if (state.filters.fileType === 'all') {
    return true;
  }

  return group.files.some((file) => normalizeFileExtension(file.extension) === state.filters.fileType);
}

function matchesFileSizeFilter(group) {
  if (state.filters.fileSize === 'all') {
    return true;
  }

  return getSizeRangeKey(group.size) === state.filters.fileSize;
}

function syncSelectedFilesToVisibleGroups() {
  const visiblePaths = new Set();
  getFilteredGroups().forEach((group) => {
    group.files.forEach((file) => {
      visiblePaths.add(file.path);
    });
  });

  state.selectedFiles = new Set(
    [...state.selectedFiles].filter((filePath) => visiblePaths.has(filePath)),
  );
}

function getAvailableFileTypeOptions() {
  const groups = state.scanResult && state.scanResult.groups ? state.scanResult.groups : [];
  const counts = new Map();

  groups.forEach((group) => {
    group.files.forEach((file) => {
      const extension = normalizeFileExtension(file.extension);
      counts.set(extension, (counts.get(extension) || 0) + 1);
    });
  });

  return [...counts.entries()]
    .sort((left, right) => {
      if (right[1] !== left[1]) {
        return right[1] - left[1];
      }

      return left[0].localeCompare(right[0], 'zh-CN');
    })
    .map(([value]) => ({
      value,
      label: value,
    }));
}

function normalizeFileExtension(extension) {
  if (!extension) {
    return '(无扩展名)';
  }

  return String(extension).trim() || '(无扩展名)';
}

function getSizeRangeKey(size) {
  const megaByte = 1024 * 1024;

  if (size < megaByte) {
    return '0-1';
  }

  if (size < megaByte * 2) {
    return '1-2';
  }

  if (size < megaByte * 3) {
    return '2-3';
  }

  if (size < megaByte * 4) {
    return '3-4';
  }

  if (size < megaByte * 5) {
    return '4-5';
  }

  if (size < megaByte * 10) {
    return '5-10';
  }

  return '10+';
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B';
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let current = bytes;
  let unitIndex = 0;

  while (current >= 1024 && unitIndex < units.length - 1) {
    current /= 1024;
    unitIndex += 1;
  }

  return `${current.toFixed(current >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(value || 0);
}

function formatDateTime(value) {
  if (!value) {
    return '-';
  }

  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(value));
}

function isImageFile(extension) {
  return ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.bmp', '.tiff', '.tif', '.heic'].includes(
    extension,
  );
}

function isVideoFile(extension) {
  return [
    '.mp4',
    '.mov',
    '.m4v',
    '.avi',
    '.mkv',
    '.flv',
    '.wmv',
    '.webm',
    '.mpeg',
    '.mpg',
    '.ts',
    '.m2ts',
    '.3gp',
    '.rmvb',
  ].includes(extension);
}

function isAudioFile(extension) {
  return [
    '.mp3',
    '.wav',
    '.flac',
    '.aac',
    '.m4a',
    '.ogg',
    '.wma',
    '.ape',
    '.alac',
    '.aiff',
    '.amr',
  ].includes(extension);
}

function toFileUrl(filePath) {
  return `${window.location.origin}/file?path=${encodeURIComponent(filePath)}`;
}

function toThumbnailUrl(filePath, size) {
  return `${window.location.origin}/thumbnail?path=${encodeURIComponent(filePath)}&size=${encodeURIComponent(size)}&v=${encodeURIComponent(state.thumbnailVersion)}`;
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function escapeAttribute(value) {
  return escapeHtml(value);
}

function waitForNextFrame() {
  return new Promise((resolve) => {
    window.requestAnimationFrame(() => resolve());
  });
}

function invalidateResultCaches(options = {}) {
  state.resultView.revision += 1;
  state.resultView.filteredGroupsRevision = -1;
  state.resultView.flatItemsRevision = -1;

  if (options.clearHeights) {
    state.resultView.heightCache.clear();
  }
}

function handleResultsViewportChange() {
  updateResultToolbarShadow();
  updateScrollToTopButton();

  if (getFlatResultItems().length <= RESULT_VIRTUALIZATION_THRESHOLD) {
    return;
  }

  scheduleResultsRender();
}

function updateResultToolbarShadow() {
  if (!elements.resultToolbar || !elements.content) {
    return;
  }

  elements.resultToolbar.classList.toggle('is-scrolled', elements.content.scrollTop > 0);
}

function updateScrollToTopButton() {
  if (!elements.scrollToTopButton || !elements.content) {
    return;
  }

  const canScroll = elements.content.scrollHeight > elements.content.clientHeight + 24;
  const shouldShow = canScroll && elements.content.scrollTop > 240;
  elements.scrollToTopButton.classList.toggle('is-visible', shouldShow);
}

function handleScrollToTop() {
  if (!elements.content) {
    return;
  }

  elements.content.scrollTo({
    top: 0,
    behavior: 'smooth',
  });
}

function scheduleResultsRender() {
  if (state.resultView.renderFrameId) {
    return;
  }

  state.resultView.renderFrameId = window.requestAnimationFrame(() => {
    state.resultView.renderFrameId = 0;
    renderResults();
  });
}

function getFlatResultItems() {
  if (state.resultView.flatItemsRevision === state.resultView.revision) {
    return state.resultView.flatItems;
  }

  const items = [];
  const groups = getFilteredGroups();

  groups.forEach((group, groupIndex) => {
    items.push({
      key: `group:${group.id}`,
      type: 'group',
      group,
      groupIndex,
    });

    if (state.collapsedGroups.has(group.id)) {
      if (group.files[0]) {
        items.push({
          key: `file:${group.files[0].path}`,
          type: 'file',
          file: group.files[0],
          isDuplicate: false,
          shouldEmphasizeDirectory: hasMultipleFileDirectories(group),
        });
      }

      items.push({
        key: `tip:${group.id}`,
        type: 'tip',
        hiddenCount: Math.max(group.files.length - 1, 0),
      });
      return;
    }

    group.files.forEach((file, fileIndex) => {
      items.push({
        key: `file:${file.path}`,
        type: 'file',
        file,
        isDuplicate: fileIndex > 0,
        shouldEmphasizeDirectory: hasMultipleFileDirectories(group),
      });
    });
  });

  state.resultView.flatItems = items;
  state.resultView.flatItemsRevision = state.resultView.revision;
  return items;
}

function getVirtualizedResultState(items) {
  if (!elements.content || items.length <= RESULT_VIRTUALIZATION_THRESHOLD) {
    return {
      virtualized: false,
      visibleItems: items,
      topPadding: 0,
      bottomPadding: 0,
    };
  }

  const contentRect = elements.content.getBoundingClientRect();
  const resultsRect = elements.results.getBoundingClientRect();
  const listTop = (resultsRect.top - contentRect.top) + elements.content.scrollTop;
  const visibleStart = Math.max(elements.content.scrollTop - listTop - RESULT_VIRTUALIZATION_OVERSCAN, 0);
  const visibleEnd = Math.max(
    visibleStart + elements.content.clientHeight + (RESULT_VIRTUALIZATION_OVERSCAN * 2),
    visibleStart,
  );

  let startIndex = 0;
  let endIndex = items.length;
  let totalHeight = 0;
  let topPadding = 0;

  for (let index = 0; index < items.length; index += 1) {
    const itemHeight = getResultItemHeight(items[index]);
    const nextHeight = totalHeight + itemHeight;

    if (nextHeight < visibleStart) {
      startIndex = index + 1;
      topPadding = nextHeight;
    }

    if (endIndex === items.length && totalHeight > visibleEnd) {
      endIndex = index;
    }

    totalHeight = nextHeight;
  }

  if (endIndex < startIndex) {
    endIndex = startIndex;
  }

  if (items.length > 0 && endIndex === startIndex) {
    endIndex = Math.min(startIndex + 1, items.length);
  }

  const visibleItems = items.slice(startIndex, endIndex);
  const visibleHeight = visibleItems.reduce((sum, item) => sum + getResultItemHeight(item), 0);

  return {
    virtualized: true,
    visibleItems,
    topPadding,
    bottomPadding: Math.max(totalHeight - topPadding - visibleHeight, 0),
  };
}

function getResultItemHeight(item) {
  return state.resultView.heightCache.get(item.key) || RESULT_ITEM_ESTIMATED_HEIGHTS[item.type] || 72;
}

function renderVirtualizedResultItems(virtualState) {
  const html = [];

  if (virtualState.virtualized && virtualState.topPadding > 0) {
    html.push(renderResultSpacer(virtualState.topPadding));
  }

  virtualState.visibleItems.forEach((item) => {
    html.push(renderResultItem(item));
  });

  if (virtualState.virtualized && virtualState.bottomPadding > 0) {
    html.push(renderResultSpacer(virtualState.bottomPadding));
  }

  return html.join('');
}

function renderResultSpacer(height) {
  return `<div class="result-virtual-spacer" style="height:${Math.round(height)}px" aria-hidden="true"></div>`;
}

function renderResultItem(item) {
  if (item.type === 'group') {
    const selectionState = getGroupSelectionState(item.group);

    return `
      <div class="result-item result-item-group" data-virtual-key="${escapeAttribute(item.key)}" data-virtual-type="group">
        <div class="result-group-title">
          <div class="group-title-main">
            <button
              class="group-collapse-button"
              type="button"
              data-group-id="${escapeAttribute(item.group.id)}"
              title="${state.collapsedGroups.has(item.group.id) ? '展开当前分组' : '收起当前分组'}"
              aria-label="${state.collapsedGroups.has(item.group.id) ? '展开当前分组' : '收起当前分组'}"
            >
              <svg
                class="${state.collapsedGroups.has(item.group.id) ? 'is-collapsed' : ''}"
                viewBox="0 0 24 24"
                fill="none"
                aria-hidden="true"
              >
                <path d="m9 6 6 6-6 6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </button>
            <label class="group-checkbox-label" title="选择当前这一组的全部文件">
              <input
                class="group-checkbox"
                type="checkbox"
                data-group-id="${escapeAttribute(item.group.id)}"
                ${selectionState.checked ? 'checked' : ''}
              />
            </label>
            <h3>第 ${item.groupIndex + 1} 组重复文件</h3>
          </div>
          <span class="tag">${formatNumber(item.group.fileCount)} 个文件</span>
        </div>
      </div>
    `;
  }

  if (item.type === 'tip') {
    return `
      <div class="result-item result-item-tip" data-virtual-key="${escapeAttribute(item.key)}" data-virtual-type="tip">
        <div class="group-collapsed-tip">
          已收起其余 ${Math.max(item.hiddenCount, 0)} 个重复文件，不影响已选文件。
        </div>
      </div>
    `;
  }

  return `
    <div class="result-item result-item-file" data-virtual-key="${escapeAttribute(item.key)}" data-virtual-type="file">
      ${renderFileRow(item.file, item.isDuplicate, { shouldEmphasizeDirectory: item.shouldEmphasizeDirectory })}
    </div>
  `;
}

function measureVisibleResultItems() {
  const renderedItems = elements.results.querySelectorAll('[data-virtual-key]');
  let didChange = false;

  renderedItems.forEach((item) => {
    const nextHeight = Math.round(item.getBoundingClientRect().height);
    if (nextHeight <= 0) {
      return;
    }

    const previousHeight = state.resultView.heightCache.get(item.dataset.virtualKey);
    if (previousHeight !== nextHeight) {
      state.resultView.heightCache.set(item.dataset.virtualKey, nextHeight);
      didChange = true;
    }
  });

  if (didChange) {
    scheduleResultsRender();
  }
}
