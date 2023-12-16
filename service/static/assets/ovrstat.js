$('.ui.dropdown').dropdown();

$(document).ready(function () {
    $('#stats-form').on('submit', function (e) {
        e.preventDefault();
        var platform = document.getElementById('platform').value;
        var type = document.getElementById('lookup').value;
        var tag = document.getElementById('tag').value;
        // Überprüfung, ob tag das Zeichen '#' enthält
        if (tag.includes('#')) {
            // Ersetzen Sie '#' durch '-'
            tag = tag.replace('#', '-');
        }
        if(type == 'profile'){
            window.open('/stats/' + platform + '/' + tag + '/profile');
        }
        if(type == 'complete'){
            window.open('/stats/' + platform + '/' + tag + '/complete');
        }
        // window.open('/stats/' + platform + '/' + tag);
    });
});