
var regexpMetric = /([a-zA-Z_:][a-zA-Z0-9_:]*)(?:{(.*)})?\s+((?:\d*\.)?\d+(?:e\d+)?)(?:\s+(\d+))?/
var regexpLabels = /([a-zA-Z_][a-zA-Z0-9_]*)\s?=\s?"([^"\\\\]*(?:\\.[^"\\\\]*)*)"/gm

function parseLabels(metricName, rawLabelsStr) {
    var labels = {"__name__": metricName};
    if (rawLabelsStr != undefined) {
        while(match = regexpLabels.exec(rawLabelsStr)){
            labels[match[1]] = match[2];
        }
    }
    return labels;
}

function parseSample(txtLine, defaultTimestampS) {
    var match = regexpMetric.exec(txtLine);
    if (match != null) {
        let labels = parseLabels(match[1], match[2]);
        let ts = parseInt(match[4]) || defaultTimestampS;
        return {"metric": labels, "value": [ts, match[3]]};
    }
    return null;
}

function handleSimulationResult(result) {
    let jsonres = JSON.parse(result);

    var html = "<dl>";
    $.each(jsonres, function(writerName, writerMsg) {
        html += '<dt>' + writerName + '</dt>'
        html += '<dd><pre class="alert alert-light">' + writerMsg + '</pre></dd>'
    });
    html += "</dl>";
    $("#outputs").html(html);
}

function simulWrite() {
    let txt = $("#input").val();
    let lines = txt.split(/\n/);
    let nowS = $.now() / 1000;

    var samples = []
    $.each(lines, function(i, line) {
        let sample = parseSample($.trim(line), nowS);
        if (sample != null) {
            samples.push(sample);
        }
    });
    $.ajax({
        url: 'write',
        type: 'post',
        data: JSON.stringify(samples),
        headers: {"Content-Type": 'application/json'},
        success: handleSimulationResult
    });
}