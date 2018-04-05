function t(str, callback) {
  $.ajax({
    type: 'GET',
    url: '/translate',
    data: {
      'str': str,
    },
    success: callback,
  });
}

function debounce(func, wait, immediate) {
  var timeout;
  return function() {
    var context = this,
      args = arguments;

    var later = function() {
      timeout = null;
      if (!immediate) func.apply(context, args);
    };

    var callNow = immediate && !timeout;
    clearTimeout(timeout);
    timeout = setTimeout(later, wait || 200);
    if (callNow) func.apply(context, args);
  };
};

function setLanguage(lang) {
  document.cookie = "obijudge-locale=" + lang;
  window.location.reload(false);
};

function setupKaTeX() {
  renderMathInElement(document.body, {
    delimiters: [{
        left: "$$",
        right: "$$",
        display: true
      },
      {
        left: "\\[",
        right: "\\]",
        display: true
      },
      {
        left: "$",
        right: "$",
        display: false
      },
      {
        left: "\\(",
        right: "\\)",
        display: false
      }
    ]
  });
};

function setupClickToCopy() {
  t('copy', function(copystr) {
    $('.testcase pre').each(function(index) {
      var id = ('test' + index.toString());
      var pre = $(this);
      pre.attr("id", id);
      pre.before($('<button class="small-button clipboard-button" clipboard-target="#' + id + '">' + copystr + '</button>'));
    });
  });

  var clipboard = new ClipboardJS('.clipboard-button', {
    text: function(trigger) {
      return document.querySelector(trigger.getAttribute('clipboard-target')).innerText;
    },
  });

  clipboard.on('success', function(e) {
    t("test_case_copied", function(str) {
      toastr.info(str);
    });
    e.clearSelection();
  });
};

function setupCodeEditor() {
  var editor = CodeMirror.fromTextArea($('textarea#code').get(0), {
    mode: "text/x-c++src",
    lineNumbers: true,
    matchBrackets: true,
    autoCloseBrackets: true,
    showCursorWhenSelecting: true,
    tabSize: 2,
    keymap: "sublime",
  });

  var container = $('.CodeMirror');
  container.after('<div class="handle">&#8801;</div>');
  var handle = $('.handle').get(0);

  handle.addEventListener("mousedown", function(e) {
    var start_y = e.y;
    var start_h = parseInt(window.getComputedStyle(container.get(0)).height.replace(/px$/, ""));

    function on_drag(e) {
      editor.setSize(null, Math.max(300, (start_h + e.y - start_y)) + "px");
    }

    function on_release(e) {
      document.body.removeEventListener("mousemove", on_drag);
      window.removeEventListener("mouseup", on_release);
    }

    document.body.addEventListener("mousemove", on_drag);
    window.addEventListener("mouseup", on_release);
  });

  $('select#lang').change(function() {
    editor.setOption("mode", this.value);
  });

  $('input#custom-input').change(function() {
    if (this.checked) $('div#customInputOutput').show();
    else $('div#customInputOutput').hide();
  });

  editor.on("changes", debounce(function(instance, changes) {
    // TODO: save code in cookies
  }, 250));
}

function setupTaskPage() {
  setupKaTeX();
  setupClickToCopy();
  setupCodeEditor();
};
