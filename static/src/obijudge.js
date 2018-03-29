$(document).ready(function() {
  renderMathInElement(document.body, {
    delimiters: [
      {left: "$$", right: "$$", display: true},
      {left: "\\[", right: "\\]", display: true},
      {left: "$", right: "$", display: false},
      {left: "\\(", right: "\\)", display: false}
    ]
  });
});

$(document).ready(function() {
  $('textarea#editor').each(function() {
    var lang = $(this).attr('lang');
    var template = $('#code_template').clone();

    // Take the code and put it into the textarea.
    template.find('textarea').val($(this).html());
    template.show();
    $(this).html(template);

    var cm = CodeMirror.fromTextArea($(this).find('textarea')[0], {
      lineNumbers: true,
      matchBrackets: true
    });

    setMode(cm, lang);
  });

  var editor = CodeMirror.fromTextArea(document.getElementById('code'), {
    mode:  "text/x-c++src",
    lineNumbers: true,
    matchBrackets: true,
  });
});

