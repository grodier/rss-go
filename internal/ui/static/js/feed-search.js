// Feed Search Progressive Enhancement
(function() {
  'use strict';

  // Configuration
  const DEBOUNCE_DELAY = 300;
  const MIN_QUERY_LENGTH = 2;

  // DOM Elements
  const searchForm = document.getElementById('search-form');
  const searchInput = document.getElementById('search-query');
  const autocompleteList = document.getElementById('autocomplete-list');
  const resultsList = document.getElementById('results-list');
  const discoveryStatus = document.getElementById('discovery-status');

  // State
  let debounceTimer = null;
  let eventSource = null;
  let selectedIndex = -1;

  // Initialize
  init();

  function init() {
    console.log('[Search] Initializing feed search');
    console.log('[Search] DOM elements:', {
      searchForm: !!searchForm,
      searchInput: !!searchInput,
      autocompleteList: !!autocompleteList,
      resultsList: !!resultsList,
      discoveryStatus: !!discoveryStatus
    });

    if (!searchForm || !searchInput) {
      console.error('[Search] Required DOM elements missing!');
      return;
    }

    // Attach event listeners
    console.log('[Search] Attaching event listeners');
    searchInput.addEventListener('input', handleInput);
    searchInput.addEventListener('keydown', handleKeyDown);
    searchForm.addEventListener('submit', handleSubmit);
    document.addEventListener('click', handleDocumentClick);
    console.log('[Search] Event listeners attached');

    // Start SSE if discovery is in progress
    if (discoveryStatus && discoveryStatus.dataset.status === 'pending') {
      const discoveryId = discoveryStatus.dataset.discoveryId;
      console.log('[Search] Discovery in progress', { discoveryId });
      if (discoveryId) {
        connectSSE(discoveryId);
      }
    }

    console.log('[Search] Initialization complete');
  }

  // Handle input changes (debounced autocomplete)
  function handleInput(e) {
    const query = e.target.value.trim();

    // Clear autocomplete if query is too short
    if (query.length < MIN_QUERY_LENGTH) {
      hideAutocomplete();
      return;
    }

    // Debounce the autocomplete request
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      fetchSuggestions(query);
    }, DEBOUNCE_DELAY);
  }

  // Handle keyboard navigation
  function handleKeyDown(e) {
    const items = autocompleteList.querySelectorAll('.autocomplete-item');

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIndex = Math.min(selectedIndex + 1, items.length - 1);
      updateSelection(items);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIndex = Math.max(selectedIndex - 1, -1);
      updateSelection(items);
    } else if (e.key === 'Enter' && selectedIndex >= 0) {
      e.preventDefault();
      items[selectedIndex].click();
    } else if (e.key === 'Escape') {
      hideAutocomplete();
    }
  }

  // Update visual selection in autocomplete
  function updateSelection(items) {
    items.forEach((item, index) => {
      if (index === selectedIndex) {
        item.classList.add('selected');
        item.scrollIntoView({ block: 'nearest' });
      } else {
        item.classList.remove('selected');
      }
    });
  }

  // Handle form submission (AJAX)
  function handleSubmit(e) {
    console.log('[Search] Form submit event triggered');
    e.preventDefault();

    const formData = new FormData(searchForm);
    const query = formData.get('q');
    const csrfToken = formData.get('csrf_token');

    console.log('[Search] Form data:', { query, csrfToken: csrfToken ? 'present' : 'missing' });

    if (!query) {
      console.warn('[Search] Query is empty, aborting');
      return;
    }

    if (!csrfToken) {
      console.error('[Search] CSRF token is missing!');
      alert('Security token missing. Please reload the page.');
      return;
    }

    // Hide autocomplete
    hideAutocomplete();

    // Show loading indicator
    showLoadingIndicator();

    console.log('[Search] Submitting search via AJAX');

    // Submit via AJAX
    submitSearch(query, csrfToken);
  }

  // Handle clicks outside autocomplete
  function handleDocumentClick(e) {
    if (!autocompleteList.contains(e.target) && e.target !== searchInput) {
      hideAutocomplete();
    }
  }

  // Fetch autocomplete suggestions
  async function fetchSuggestions(query) {
    try {
      const response = await fetch(`/feeds/suggest?q=${encodeURIComponent(query)}`);
      if (!response.ok) throw new Error('Failed to fetch suggestions');

      const suggestions = await response.json();
      displaySuggestions(suggestions);
    } catch (error) {
      console.error('Autocomplete error:', error);
      hideAutocomplete();
    }
  }

  // Display autocomplete suggestions
  function displaySuggestions(suggestions) {
    if (!suggestions || suggestions.length === 0) {
      hideAutocomplete();
      return;
    }

    autocompleteList.innerHTML = '';
    selectedIndex = -1;

    suggestions.forEach((item) => {
      const div = document.createElement('div');
      div.className = 'autocomplete-item';
      div.innerHTML = `
        <strong>${escapeHtml(item.Title)}</strong><br>
        <small>${escapeHtml(item.SiteURL || item.FeedURL)}</small>
      `;
      div.addEventListener('click', () => {
        searchInput.value = item.Title;
        hideAutocomplete();
        searchForm.dispatchEvent(new Event('submit'));
      });
      autocompleteList.appendChild(div);
    });

    autocompleteList.classList.add('active');
  }

  // Hide autocomplete
  function hideAutocomplete() {
    autocompleteList.classList.remove('active');
    autocompleteList.innerHTML = '';
    selectedIndex = -1;
  }

  // Submit search via AJAX
  async function submitSearch(query, csrfToken) {
    console.log('[Search] Starting AJAX submission', { query });

    try {
      const formData = new FormData();
      formData.append('q', query);
      formData.append('csrf_token', csrfToken);

      console.log('[Search] Sending POST request to /feeds/search');

      const response = await fetch('/feeds/search', {
        method: 'POST',
        headers: {
          'Accept': 'application/json',
        },
        body: formData,
      });

      console.log('[Search] Response received', {
        status: response.status,
        ok: response.ok,
        contentType: response.headers.get('Content-Type')
      });

      if (!response.ok) {
        const text = await response.text();
        console.error('[Search] Request failed', { status: response.status, body: text });
        throw new Error(`Search failed with status ${response.status}`);
      }

      const data = await response.json();
      console.log('[Search] Response data:', data);

      // Hide loading indicator
      hideLoadingIndicator();

      // Update URL without reload
      const url = new URL(window.location);
      url.searchParams.set('q', query);
      if (data.discovery_id) {
        url.searchParams.set('d', data.discovery_id);
      } else {
        url.searchParams.delete('d');
      }
      window.history.pushState({}, '', url);

      // Display results
      console.log('[Search] Displaying results', { count: data.results?.length || 0 });
      displayResults(data.results);

      // Connect to SSE if discovery started
      if (data.discovery_id) {
        console.log('[Search] Starting discovery SSE', { discoveryId: data.discovery_id });
        showDiscoveryStatus('pending', 'Discovering feeds...');
        connectSSE(data.discovery_id);
      } else {
        console.log('[Search] No discovery triggered');
        hideDiscoveryStatus();
      }
    } catch (error) {
      console.error('[Search] Error during search:', error);
      hideLoadingIndicator();
      alert('Search failed: ' + error.message);
    }
  }

  // Show loading indicator
  function showLoadingIndicator() {
    console.log('[Search] Showing loading indicator');
    const button = searchForm.querySelector('button[type="submit"]');
    if (button) {
      button.disabled = true;
      button.textContent = 'Searching...';
    }
  }

  // Hide loading indicator
  function hideLoadingIndicator() {
    console.log('[Search] Hiding loading indicator');
    const button = searchForm.querySelector('button[type="submit"]');
    if (button) {
      button.disabled = false;
      button.textContent = 'Search';
    }
  }

  // Display search results
  function displayResults(results) {
    console.log('[Search] displayResults called', {
      results: results,
      resultsLength: results?.length,
      resultsListExists: !!resultsList
    });

    if (!resultsList) {
      console.error('[Search] resultsList element not found!');
      return;
    }

    const resultsSection = document.getElementById('results-section');
    const emptyState = document.getElementById('empty-state');
    const resultsHeader = document.getElementById('results-header');

    if (!results || results.length === 0) {
      console.log('[Search] No results, showing empty state');

      // Hide results list
      resultsList.style.display = 'none';
      resultsList.innerHTML = '';

      // Hide results header if it exists
      if (resultsHeader) {
        resultsHeader.style.display = 'none';
      }

      // Show or create empty state
      if (emptyState) {
        emptyState.style.display = 'block';
        emptyState.innerHTML = '<p>No results found</p>';
      } else {
        const newEmptyState = document.createElement('div');
        newEmptyState.id = 'empty-state';
        newEmptyState.className = 'empty-state';
        newEmptyState.innerHTML = '<p>No results found</p>';
        resultsSection.insertBefore(newEmptyState, resultsList);
      }

      return;
    }

    console.log('[Search] Clearing existing results and adding new ones');

    // Hide empty state
    if (emptyState) {
      emptyState.style.display = 'none';
    }

    // Show and update results header
    if (resultsHeader) {
      resultsHeader.textContent = `Results (${results.length})`;
      resultsHeader.style.display = 'block';
    } else {
      const newHeader = document.createElement('h2');
      newHeader.id = 'results-header';
      newHeader.textContent = `Results (${results.length})`;
      resultsSection.insertBefore(newHeader, resultsList);
    }

    // Show and populate results list
    resultsList.style.display = 'block';
    resultsList.innerHTML = '';

    results.forEach((result, index) => {
      console.log(`[Search] Adding result ${index + 1}:`, result);
      const li = document.createElement('li');
      li.className = 'result-item';
      li.innerHTML = `
        <h3 class="result-title">${escapeHtml(result.Title)}</h3>
        <p class="result-url">
          <strong>Feed:</strong> <a href="${escapeHtml(result.FeedURL)}" target="_blank" rel="noopener noreferrer">${escapeHtml(result.FeedURL)}</a>
        </p>
        ${result.SiteURL ? `
          <p class="result-url">
            <strong>Site:</strong> <a href="${escapeHtml(result.SiteURL)}" target="_blank" rel="noopener noreferrer">${escapeHtml(result.SiteURL)}</a>
          </p>
        ` : ''}
        <span class="result-source ${result.Source}">${result.Source}</span>
        ${result.Reason ? `<p class="result-reason">${escapeHtml(result.Reason)}</p>` : ''}
      `;
      resultsList.appendChild(li);
    });

    console.log(`[Search] Successfully added ${results.length} results to DOM`);
  }

  // Show discovery status
  function showDiscoveryStatus(status, message) {
    console.log('[Search] showDiscoveryStatus called', {
      status,
      message,
      discoveryStatusExists: !!discoveryStatus
    });

    if (!discoveryStatus) {
      console.error('[Search] discoveryStatus element not found!');
      return;
    }

    discoveryStatus.className = `discovery-status ${status}`;
    discoveryStatus.dataset.status = status;

    let icon = '';
    if (status === 'pending') {
      icon = '<span class="spinner"></span>';
    } else if (status === 'resolved_found') {
      icon = '✓ ';
    }

    discoveryStatus.innerHTML = `
      <h3>${icon}${escapeHtml(message)}</h3>
    `;
    discoveryStatus.style.display = 'block';
    console.log('[Search] Discovery status shown');
  }

  // Hide discovery status
  function hideDiscoveryStatus() {
    console.log('[Search] hideDiscoveryStatus called', {
      discoveryStatusExists: !!discoveryStatus
    });

    if (!discoveryStatus) return;
    discoveryStatus.style.display = 'none';
    console.log('[Search] Discovery status hidden');
  }

  // Connect to SSE for discovery updates
  function connectSSE(discoveryId) {
    console.log('[SSE] Connecting to SSE', { discoveryId });

    // Close existing connection
    if (eventSource) {
      console.log('[SSE] Closing existing connection');
      eventSource.close();
    }

    const url = `/feeds/discover/${discoveryId}/events`;
    console.log('[SSE] Creating EventSource', { url });

    try {
      eventSource = new EventSource(url);

      eventSource.onopen = () => {
        console.log('[SSE] Connection opened successfully');
      };

      eventSource.addEventListener('progress', (e) => {
        console.log('[SSE] Progress event received', { data: e.data, lastEventId: e.lastEventId });
        try {
          const data = JSON.parse(e.data);
          console.log('[SSE] Parsed progress data', data);
          showDiscoveryStatus('pending', data.message);
        } catch (error) {
          console.error('[SSE] Failed to parse progress event data', error);
        }
      });

      eventSource.addEventListener('results', (e) => {
        console.log('[SSE] Results event received', { data: e.data, lastEventId: e.lastEventId });
        try {
          const data = JSON.parse(e.data);
          console.log('[SSE] Parsed results data', data);
          if (data.results && data.results.length > 0) {
            console.log('[SSE] Appending results', { count: data.results.length });
            appendResults(data.results);
          }
        } catch (error) {
          console.error('[SSE] Failed to parse results event data', error);
        }
      });

      eventSource.addEventListener('done', (e) => {
        console.log('[SSE] Done event received', { data: e.data, lastEventId: e.lastEventId });
        try {
          const data = JSON.parse(e.data);
          console.log('[SSE] Parsed done data', data);
          showDiscoveryStatus('resolved_found', data.message);
          console.log('[SSE] Closing connection - discovery complete');
          eventSource.close();
          eventSource = null;
        } catch (error) {
          console.error('[SSE] Failed to parse done event data', error);
        }
      });

      eventSource.addEventListener('error', (e) => {
        console.log('[SSE] Error event received', { event: e, data: e.data });
        try {
          const data = e.data ? JSON.parse(e.data) : { message: 'Discovery failed' };
          console.log('[SSE] Parsed error data', data);
          showDiscoveryStatus('error', data.message);
          eventSource.close();
          eventSource = null;
        } catch (error) {
          console.error('[SSE] Failed to parse error event data', error);
          showDiscoveryStatus('error', 'Discovery failed');
          eventSource.close();
          eventSource = null;
        }
      });

      eventSource.onerror = (e) => {
        console.error('[SSE] Connection error', {
          readyState: eventSource.readyState,
          event: e
        });

        // ReadyState values: 0=CONNECTING, 1=OPEN, 2=CLOSED
        if (eventSource.readyState === 2) {
          console.error('[SSE] Connection closed, not attempting reconnect');
          eventSource.close();
          eventSource = null;
        }
      };

      console.log('[SSE] Event listeners attached');
    } catch (error) {
      console.error('[SSE] Failed to create EventSource', error);
    }
  }

  // Append new results to the list
  function appendResults(newResults) {
    console.log('[Search] appendResults called', {
      newResults,
      count: newResults?.length,
      resultsListExists: !!resultsList
    });

    if (!resultsList) {
      console.error('[Search] resultsList not found in appendResults');
      return;
    }

    // Show results list if hidden
    resultsList.style.display = 'block';

    // Hide empty state if visible
    const emptyState = document.getElementById('empty-state');
    if (emptyState) {
      emptyState.style.display = 'none';
    }

    newResults.forEach((result, index) => {
      console.log(`[Search] Appending discovered result ${index + 1}:`, result);
      const li = document.createElement('li');
      li.className = 'result-item';
      li.innerHTML = `
        <h3 class="result-title">${escapeHtml(result.Title)}</h3>
        <p class="result-url">
          <strong>Feed:</strong> <a href="${escapeHtml(result.FeedURL)}" target="_blank" rel="noopener noreferrer">${escapeHtml(result.FeedURL)}</a>
        </p>
        ${result.SiteURL ? `
          <p class="result-url">
            <strong>Site:</strong> <a href="${escapeHtml(result.SiteURL)}" target="_blank" rel="noopener noreferrer">${escapeHtml(result.SiteURL)}</a>
          </p>
        ` : ''}
        <span class="result-source ${result.Source}">${result.Source}</span>
        ${result.Reason ? `<p class="result-reason">${escapeHtml(result.Reason)}</p>` : ''}
      `;
      resultsList.appendChild(li);
    });

    console.log(`[Search] Successfully appended ${newResults.length} discovered results`);
  }

  // Utility: Escape HTML
  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }
})();
