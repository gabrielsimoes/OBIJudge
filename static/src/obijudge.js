const Result = {
  Nothing: 0,
  Timeout: 1,
  Signal: 2,
  Failed: 3,
  Correct: 4,
  Wrong: 5,
};

const ResultComp = {
  Nothing: 0,
  Timeout: 1,
  Signal: 2,
  Failed: 3,
  Success: 4,
};

function t(str, callback) {
  $.ajax({
    type: 'GET',
    url: '/translate',
    data: {
      'str': str,
    },
    success: callback,
  });
};

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

function getTaskName() {
  return document.location.pathname.split("/")[2];
};

function getResult(url, params, callback, cnt) {
  if (cnt > 1200) return; // 10 min

  $.get(url, params, function(data) {
    if (data.length > 0)
      callback(data[0]);
    else
      setTimeout(function() {
        getResult(url, params, callback, cnt + 1);
      }, 500);
  }, "json");
}

function setLanguage(lang) {
  document.cookie = "obijudge-locale=" + lang + ";path=/";
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

function setupTestCaseCopy() {
  t('copy', function(copystr) {
    $('.testcase pre').each(function(index) {
      id = ('test' + index.toString());
      $(this).attr("id", id);

      btn = $('<button class="small-button copy-testcase-button clipboard-button" clipboard-target="#' + id + '">' + copystr + '</button>');
      $(this).before(btn);
    });
  });
};

function setupClickToCopy() {
  var clipboard = new ClipboardJS('.clipboard-button', {
    text: function(trigger) {
      return document.querySelector(trigger.getAttribute('clipboard-target')).innerText;
    },
  });

  t('copied', function(copiedstr) {
    clipboard.on('success', function(e) {
      toastr.options.timeOut = 1000;
      toastr.info(copiedstr);
      e.clearSelection();
    });
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

  $('input#custom-input').change(function() {
    if (this.checked) {
      $('div#customInputOutput').show();
      $('div#test-info').show();
    } else {
      $('div#customInputOutput').hide();
      $('div#test-info').hide();
    }
  });

  var form = $('form#submission-form');
  form.submit(function(e) {
    e.preventDefault();

    var data = new FormData(form[0]);
    data.append('file', $('#file')[0].files[0]);

    var isTest = $('#custom-input').is(':checked')

    $.ajax({
      url: (isTest ? '/test/' : '/submit/') + getTaskName(),
      type: 'POST',
      data: data,
      processData: false,
      contentType: false,
      success: function(data) {
        data = JSON.parse(data)

        t("submission_sent", function(str) {
          toastr.success(str);
        });

        if (isTest) {
          $('#loading-test').show()
          $('#test-info').hide()

          getResult('/gettest', {
            id: data.ID,
          }, function(result) {
            $('#loading-test').hide();
            $('#test-info').show()
            formatTest(result);
          });
        } else {
          appendSubmission(data.ID);
        }
      },
      error: function(data) {
        data = JSON.parse(data.responseText)
        t("error", function(str) {
          toastr.error(str + ": " + data.Error);
        });
      },
    });
  });

  langSelect = $('select#lang')

  langSelect.change(function() {
    editor.setOption("mode", this.options[this.selectedIndex].getAttribute('mime'))
  });

  $.get('/getcode', {
    task: getTaskName(),
  }, function(data) {
    langSelect.val(data.Lang);
    editor.setOption("mode", langSelect[0].options[langSelect[0].selectedIndex].getAttribute('mime'))
    editor.setValue(data.Code);
  }, "json");

  editor.on("changes", debounce(function(instance, changes) {
    $.post('/setcode', {
      task: getTaskName(),
      code: editor.getValue(),
      lang: $('select#lang').val(),
    }, null, "json");
  }, 250));
};

function formatTime(data) {
  return moment(data.When).format('LTS');
};

function formatDuration(duration) {
  if (duration == 0) return "-";
  else return (duration / 10 ** 6).toFixed(1) + " ms";
}

function formatBatchesDuration(data) {
  if (data.Batches == null) return "-"

  var duration = 0;
  for (var i = 0; i < data.Batches.length; i++) {
    if (data.Batches[i].Time > duration) {
      duration = data.Batches[i].Time;
    }
  }

  return formatDuration(duration);
};

function formatMemory(kb) {
  if (kb == 0)
    return "-"
  else if (kb < (1 << 10))
    return kb + " KB";
  else if (kb < (1 << 20))
    return (kb / (1 << 10)).toFixed(2) + " MB";
  else
    return (kb / (1 << 20)).toFixed(2) + " GB";
};

function formatBatchesMemory(data) {
  if (data.Batches == null) return "-"

  var kb = 0;
  for (var i = 0; i < data.Batches.length; i++) {
    if (data.Batches[i].Memory > kb) {
      kb = data.Batches[i].Memory;
    }
  }

  return formatMemory(kb)
};

function formatCompilationKey(data) {
  if (data == ResultComp.Timeout) {
    return "result_comp_timeout"
  } else if (data == ResultComp.Signal) {
    return "result_comp_signal"
  } else if (data == ResultComp.Failed) {
    return "result_comp_failed"
  } else {
    return "result_comp_success"
  }
};

function formatResultKey(data) {
  if (data == Result.Timeout) {
    return "result_timeout"
  } else if (data == Result.Signal) {
    return "result_signal"
  } else if (data == Result.Failed) {
    return "result_failed"
  } else if (data == Result.Wrong) {
    return "result_wrong"
  } else {
    return "result_correct"
  }
};

function formatResult(data, tag) {
  if (data == null) {
    tag.text('-');
    return
  }

  var key = ""
  tag.css("text-decoration", "underline");

  if (data.Error) {
    key = "error"
  } else {
    key = formatCompilationKey(data.Compilation)

    if (key == "result_comp_success") {
      if (data.Batches != undefined) {
        for (var i = 0; i < data.Batches.length; i++) {
          key = formatResultKey(data.Batches[i].Result);
          if (key != "result_correct") break;
        }
      }

      if (data.Result != undefined) {
        key = formatResultKey(data.Result);
      }
    }
  }

  t(key, function(resultstr) {
    tag.text(resultstr);
    tag.css("color", (key == "result_correct") ? "green" : "red");
  });
};

function formatCompilationExtra(data, tag) {
  key = formatCompilationKey(data.Compilation);

  var resultDiv = $('<div></div>');
  var explanationDiv = $('<div></div>');
  var extraDiv = $('<div></div>');

  t(key, function(result) {
    resultDiv.text(result);
    resultDiv.css("color", (key == "result_comp_success") ? "green" : "red");
  });
  tag.append(resultDiv);

  t("explanation_" + key, function(explanation) {
    explanationDiv.html(explanation.replace(/\n/g, '<br />'));
  });
  tag.append(explanationDiv);

  if (data.Extra.length > 0) tag.append('<br />');
  extraDiv.html(data.Extra.replace(/\n/g, '<br />'));
  tag.append(extraDiv);
};

function formatResultExtra(result, batch, score, time, memory, extra, tag) {
  tag.append('<br />');

  var key = formatResultKey(result);

  var batchDiv = $('<div></div>');
  var resultDiv = $('<div></div>');
  var explanationDiv = $('<div></div>');
  var scoreDiv = $('<div></div>');
  var timeDiv = $('<div></div>');
  var memoryDiv = $('<div></div>');
  var extraDiv = $('<div></div>');

  if (batch != -1) {
    t("batch", function(batchstr) {
      batchDiv.text(batchstr + ': ' + batch);
    });
    tag.append(batchDiv);
  }

  t(key, function(result) {
    resultDiv.text(result);
    resultDiv.css("color", (key == "result_correct") ? "green" : "red");
  });
  tag.append(resultDiv);

  t("explanation_" + key, function(explanation) {
    explanationDiv.html(explanation.replace(/\n/g, '<br />'));
  });
  tag.append(explanationDiv);

  if (score != -1) {
    t('score', function(scorestr) {
      scoreDiv.text(scorestr + ': ' + score);
    });
    tag.append(scoreDiv);
  }

  t('time', function(timestr) {
    timeDiv.text(timestr + ': ' + formatDuration(time));
  });
  tag.append(timeDiv);

  t('memory', function(memorystr) {
    memoryDiv.text(memorystr + ': ' + formatMemory(memory));
  });
  tag.append(memoryDiv);

  if (extra.length > 0) tag.append('<br />');
  extraDiv.html(extra.replace(/\n/g, '<br />'));
  tag.append(extraDiv);
};

function formatBatchesResult(data, tag) {
  if (data == null) {
    return
  }

  for (var i = 0; i < data.length; i++) {
    formatResultExtra(data[i].Result, i, data[i].Score, data[i].Time, data[i].Memory, data[i].Extra, tag);
  }
};

function formatErrorExtra(error, tag) {
  var resultDiv = $('<div></div>');
  var explanationDiv = $('<div></div>');
  var extraDiv = $('<div></div>');

  t("error", function(errstr) {
    resultDiv.text(errstr);
    resultDiv.css("color", "red");
  });
  tag.append(resultDiv);

  t("explanation_error", function(explanation) {
    explanationDiv.html(explanation.replace(/\n/g, '<br />'));
  });
  tag.append(explanationDiv);

  if (error.length > 0) tag.append('<br />');
  extraDiv.html(error.replace(/\n/g, '<br />'));
  tag.append(extraDiv);
}

function formatExtra(data, tag) {
  tag.html('');
  tag.css("text-align", "left");

  if (data == null) {
    tag.text('-');
    return
  }

  if (data.Error) {
    formatErrorExtra(data.Extra, tag);
    return
  }

  compDiv = $('<div></div>');
  formatCompilationExtra(data, compDiv);
  tag.append(compDiv);

  if (data.Compilation == ResultComp.Success) {
    if (data.Batches != undefined) {
      batchesDiv = $('<div></div>');
      formatBatchesResult(data.Batches, batchesDiv);
      tag.append(batchesDiv);
    }

    if (data.Result != undefined) {
      resultDiv = $('<div></div>');
      formatResultExtra(data.Result, -1, -1, data.Time, data.Memory, data.Extra, resultDiv);
      tag.append(resultDiv);
    }
  }
};

function formatBatchesScore(data) {
  if (data.Batches == null || data.Batches.length == 0) {
    return "-"
  }

  var score = 0;
  for (var i = 0; i < data.Batches.length; i++) {
    score += data.Batches[i].Score;
  }
  return score;
};

var testInfoExtra = document.createElement('div');

function setupTestTippy() {
  var resultSpan = $('span#test-info-result')
  tippy.one(resultSpan[0], {
    html: testInfoExtra,
    theme: 'light',
    arrow: true,
    distance: 0,
    interactive: true,
  });
};

function formatTest(data) {
  var resultSpan = $('span#test-info-result')
  var timeSpan = $('span#test-info-time')
  var memorySpan = $('span#test-info-memory')

  formatResult(data, resultSpan);
  timeSpan.html(formatDuration(data.Time));
  memorySpan.html(formatMemory(data.Memory));

  formatExtra(data, $(testInfoExtra));

  $('#output').text(data.Output);
};

function formatSubmission(data, row) {
  var td = '<td></td>'
  var timeTd = $(td),
    resultTd = $(td),
    scoreTd = $(td),
    durationTd = $(td),
    memoryTd = $(td),
    langTd = $(td);
  row.append(timeTd).append(resultTd).append(scoreTd)
    .append(durationTd).append(memoryTd).append(langTd);

  timeTd.html(formatTime(data));
  formatResult(data, resultTd);
  scoreTd.html(formatBatchesScore(data));
  durationTd.html(formatBatchesDuration(data));
  memoryTd.html(formatBatchesMemory(data));
  langTd.text(data.LangName);

  langTd.css("text-decoration", "underline");
  var codeTippy = $(document.createElement('div'));
  t("copy_code", function(copycodestr) {
    copyBtn = $('<button class="clipboard-button" clipboard-target="#code-' + data.ID + '">' + copycodestr + '</button>');
    codeTippy.append(copyBtn);

    var codePre = $('<pre id="code-' + data.ID + '" class="cm-s-default"></pre>');
    codePre.css("text-align", "left");
    CodeMirror.runMode(data.Code, data.LangMime, codePre[0]);
    codeTippy.append(codePre);

    tippy.one(langTd[0], {
      html: codeTippy[0],
      theme: 'light',
      arrow: true,
      distance: 0,
      performance: true,
      interactive: true,
    });
  });

  var extra = $(document.createElement('div'));
  tippy.one(resultTd[0], {
    html: extra[0],
    theme: 'light',
    arrow: true,
    distance: 0,
    performance: true,
    interactive: true,
  });
  formatExtra(data, extra);
};

function formatOverviewSubmission(data, row) {
  var td = '<td></td>'
  var resultTd = $(td),
    scoreTd = $(td);
  row.append(resultTd).append(scoreTd);

  if (data.length == 0) {
    resultTd.text("-");
    scoreTd.text("-");
    return;
  }

  data = data[data.length - 1];

  formatResult(data, resultTd);
  scoreTd.html(formatBatchesScore(data));

  var extra = $(document.createElement('div'));
  tippy.one(resultTd[0], {
    html: extra[0],
    theme: 'light',
    arrow: true,
    distance: 0,
    performance: true,
    interactive: true,
  });
  formatExtra(data, extra);
};

function appendSubmission(id) {
  var tbody = $('#submissions-table').children('tbody');
  var row = $("<tr></tr>");
  tbody.append(row);

  row.html('<td colspan="6"><div class="loading"></div></td>');

  getResult('/getsubmission', {
    id: id,
  }, function(result) {
    row.html('');
    formatSubmission(result, row);
  });
};

function setupSubmissions() {
  table = $('#submissions-table')
  tbody = table.children('tbody')

  $.get('/getsubmission', {
    task: getTaskName()
  }, function(data) {
    $.each(data, function(index, submission) {
      var row = $("<tr></tr>");
      tbody.append(row);
      formatSubmission(submission, row);
    });
  }, "json");
};

function setupOverviewSubmissions() {
  table = $('#submissions-table')
  tbody = table.children('tbody')

  $.get('/gettasks', {}, function(data) {
    $.each(data, function(index, task) {
      var row = $("<tr></tr>");
      tbody.append(row);
      row.append("<td>" + task.Title + "</td>");
      $.get('/getsubmission', {
        task: task.Name,
      }, function(submissions) {
        formatOverviewSubmission(submissions, row);
      }, "json");
    });
  }, "json");
};

function setupTaskPage() {
  setupKaTeX();
  setupClickToCopy();
  setupTestCaseCopy();
  setupCodeEditor();
  setupTestTippy();
  setupSubmissions();
};

function setupOverviewPage() {
  setupOverviewSubmissions();
};
