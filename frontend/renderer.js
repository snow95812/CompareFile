const state = {
  directories: [],
  scanResult: null,
  isScanning: false,
  isDeleting: false,
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
    stage: '未删除',
    detail: '',
  },
};

const elements = {
  appTitle: document.getElementById('appTitle'),
  selectDirectoriesButton: document.getElementById('selectDirectoriesButton'),
  scanButton: document.getElementById('scanButton'),
  clearButton: document.getElementById('clearButton'),
  selectAllButton: document.getElementById('selectAllButton'),
  invertSelectButton: document.getElementById('invertSelectButton'),
  autoSelectButton: document.getElementById('autoSelectButton'),
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
  deleteProgressStage: document.getElementById('deleteProgressStage'),
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
  skippedCount: document.getElementById('skippedCount'),
  skippedList: document.getElementById('skippedList'),
};

elements.selectDirectoriesButton.addEventListener('click', handleSelectDirectories);
elements.scanButton.addEventListener('click', handleScan);
elements.clearButton.addEventListener('click', handleClearDirectories);
elements.selectAllButton.addEventListener('click', handleSelectAllFiles);
elements.invertSelectButton.addEventListener('click', handleInvertSelectedFiles);
elements.autoSelectButton.addEventListener('click', handleAutoSelectDuplicates);
elements.deleteButton.addEventListener('click', handleDeleteSelectedFiles);
elements.fileTypeFilter.addEventListener('change', handleFilterChange);
elements.fileSizeFilter.addEventListener('change', handleFilterChange);
elements.results.addEventListener('change', handleResultsChange);
elements.results.addEventListener('click', handleResultsClick);

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
      stage: progress.stage || '处理中',
      detail: progress.detail || '',
    };
    renderDeleteProgressModal();
  });
}

initializeAppTitle();
render();

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
  state.scanResult = null;
  state.selectedFiles = new Set();
  state.collapsedGroups = new Set();
  state.scanProgress = {
    percent: 0,
    stage: '准备扫描目录',
    detail: '',
  };
  render();

  try {
    state.scanResult = await window.duplicateFinderAPI.scanDirectories(state.directories);
  } catch (error) {
    state.scanResult = {
      error: error.message || '扫描失败，请稍后再试。',
    };
  } finally {
    state.isScanning = false;
    render();
  }
}

function handleClearDirectories() {
  state.directories = [];
  state.scanResult = null;
  state.selectedFiles = new Set();
  state.collapsedGroups = new Set();
  render();
}

function handleRemoveDirectory(directory) {
  state.directories = state.directories.filter((item) => item !== directory);
  state.scanResult = null;
  state.selectedFiles = new Set();
  state.collapsedGroups = new Set();
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
  state.deleteProgress = {
    percent: 0,
    stage: '准备删除文件',
    detail: `共 ${selectedPaths.length} 个文件`,
  };
  render();

  try {
    const result = await window.duplicateFinderAPI.deleteFiles(selectedPaths);
    const currentGroups = state.scanResult && state.scanResult.groups ? state.scanResult.groups : [];
    const currentSummary = state.scanResult && state.scanResult.summary ? state.scanResult.summary : null;
    const currentSkipped = state.scanResult && state.scanResult.skipped ? state.scanResult.skipped : [];

    if (result.failed.length > 0) {
      const groups = filterDeletedFilesFromGroups(currentGroups, result.deleted);
      state.scanResult = {
        ...(state.scanResult || {}),
        error: `已删除 ${result.deleted.length} 个文件，${result.failed.length} 个删除失败。`,
        groups,
        summary: buildSummaryFromGroups(currentSummary, groups, result.deleted.length),
        skipped: [
          ...currentSkipped,
          ...result.failed,
        ],
      };
    } else {
      const groups = filterDeletedFilesFromGroups(currentGroups, result.deleted);
      state.scanResult = {
        ...(state.scanResult || {}),
        groups,
        summary: buildSummaryFromGroups(currentSummary, groups, result.deleted.length),
      };
    }

    state.selectedFiles = new Set();
  } catch (error) {
    state.scanResult = {
      ...(state.scanResult || {}),
      error: error.message || '删除失败，请稍后再试。',
    };
  } finally {
    state.isDeleting = false;
    render();
  }
}

function handleFilterChange() {
  state.filters = {
    fileType: elements.fileTypeFilter.value,
    fileSize: elements.fileSizeFilter.value,
  };
  syncSelectedFilesToVisibleGroups();
  render();
}

function render() {
  renderDirectoryList();
  renderScanState();
  renderScanProgress();
  renderDeleteProgressModal();
  renderSummary();
  renderResults();
  renderSkipped();
  updateButtons();
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
    elements.statusText.textContent = '正在扫描文件并计算重复结果，请稍候...';
    elements.scanState.textContent = '处理中';
    elements.scanState.className = 'scan-state running';
    return;
  }

  if (state.isDeleting) {
    elements.statusText.textContent = '正在删除已选文件，请稍候...';
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
    elements.deleteProgressBar.style.width = '0%';
    return;
  }

  const percent = Math.max(0, Math.min(100, state.deleteProgress.percent || 0));
  elements.deleteProgressModal.classList.remove('is-hidden');
  elements.deleteProgressModal.setAttribute('aria-hidden', 'false');
  elements.deleteProgressStage.textContent = state.deleteProgress.stage || '正在删除文件';
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
  elements.resultCount.textContent = `${groups.length} 组`;
  elements.selectedCount.textContent = `已选 ${state.selectedFiles.size} 个`;
  elements.fileTypeFilter.value = state.filters.fileType;
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

  if (groups.length === 0) {
    elements.results.innerHTML = state.scanResult && state.scanResult.groups && state.scanResult.groups.length
      ? '<div class="empty-state">当前筛选条件下没有匹配结果。</div>'
      : '<div class="empty-state">当前没有找到完全相同的文件。</div>';
    return;
  }

  elements.results.innerHTML = groups
    .map(
      (group, index) => `
        <section class="result-group">
          <div class="result-group-title">
            <div class="group-title-main">
              <button
                class="group-collapse-button"
                type="button"
                data-group-id="${escapeAttribute(group.id)}"
                title="${state.collapsedGroups.has(group.id) ? '展开当前分组' : '收起当前分组'}"
                aria-label="${state.collapsedGroups.has(group.id) ? '展开当前分组' : '收起当前分组'}"
              >
                <svg
                  class="${state.collapsedGroups.has(group.id) ? 'is-collapsed' : ''}"
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
                  data-group-id="${escapeAttribute(group.id)}"
                  ${getGroupSelectionState(group).checked ? 'checked' : ''}
                />
              </label>
              <h3>第 ${index + 1} 组重复文件</h3>
            </div>
            <span class="tag">${formatNumber(group.fileCount)} 个文件</span>
          </div>
          ${state.collapsedGroups.has(group.id)
            ? `
                ${group.files[0] ? renderFileRow(group.files[0], false) : ''}
                <div class="group-collapsed-tip">
                  已收起其余 ${Math.max(group.files.length - 1, 0)} 个重复文件，不影响已选文件。
                </div>
              `
            : group.files
                .map((file, fileIndex) => renderFileRow(file, fileIndex > 0))
                .join('')}
        </section>
      `,
    )
    .join('');

  elements.results.querySelectorAll('.group-checkbox').forEach((checkbox) => {
    const group = groups.find((item) => item.id === checkbox.dataset.groupId);
    if (!group) {
      return;
    }

    checkbox.indeterminate = getGroupSelectionState(group).indeterminate;
  });
}

function renderFileRow(file, isDuplicate) {
  const checked = state.selectedFiles.has(file.path);

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
          <span class="support-item">位置：${escapeHtml(file.path)}</span>
        </div>
      </div>
      <div class="action-cell">
        <button
          class="icon-button reveal-button"
          type="button"
          data-path="${escapeAttribute(file.path)}"
          title="在 Finder 中定位文件"
          aria-label="在 Finder 中定位文件"
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
        src="${escapeAttribute(toFileUrl(file.path))}"
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
    await window.duplicateFinderAPI.previewFile(previewButton.dataset.path);
    return;
  }

  const revealButton = event.target.closest('.reveal-button');
  if (revealButton) {
    event.stopPropagation();
    await window.duplicateFinderAPI.revealFile(revealButton.dataset.path);
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

function updateButtons() {
  const isBusy = state.isScanning || state.isDeleting;
  const scanDisabled = state.directories.length === 0 || isBusy;
  const hasScanGroups = Boolean(state.scanResult && state.scanResult.groups && state.scanResult.groups.length);
  const hasVisibleGroups = getFilteredGroups().length > 0;

  elements.scanButton.disabled = scanDisabled;
  elements.clearButton.disabled = state.directories.length === 0 || isBusy;
  elements.selectDirectoriesButton.disabled = isBusy;
  elements.selectAllButton.disabled = !hasVisibleGroups || isBusy;
  elements.invertSelectButton.disabled = !hasVisibleGroups || isBusy;
  elements.autoSelectButton.disabled = !hasVisibleGroups || isBusy;
  elements.deleteButton.disabled = state.selectedFiles.size === 0 || isBusy;
  elements.fileTypeFilter.disabled = !hasScanGroups || isBusy;
  elements.fileSizeFilter.disabled = !hasScanGroups || isBusy;
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
  const groups = state.scanResult && state.scanResult.groups ? state.scanResult.groups : [];

  return groups.filter((group) => {
    const matchesType = matchesFileTypeFilter(group);
    const matchesSize = matchesFileSizeFilter(group);
    return matchesType && matchesSize;
  });
}

function matchesFileTypeFilter(group) {
  if (state.filters.fileType === 'all') {
    return true;
  }

  const category = getFileCategory(group.extension || (group.files && group.files[0] && group.files[0].extension));
  return category === state.filters.fileType;
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

function getFileCategory(extension) {
  if (isImageFile(extension)) {
    return 'image';
  }

  if (isVideoFile(extension)) {
    return 'video';
  }

  if (isAudioFile(extension)) {
    return 'audio';
  }

  return 'other';
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

  return '5+';
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
