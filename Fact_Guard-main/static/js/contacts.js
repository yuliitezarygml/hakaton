document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('.contact-copy-field').forEach(function (el) {
        el.addEventListener('click', function () {
            const value = el.getAttribute('data-copy');
            if (navigator.clipboard) {
                navigator.clipboard.writeText(value);
            } else {
                const temp = document.createElement('input');
                temp.value = value;
                document.body.appendChild(temp);
                temp.select();
                document.execCommand('copy');
                document.body.removeChild(temp);
            }
            // Показать уведомление
            showCopyNotification();
            el.classList.add('copied');
            setTimeout(() => el.classList.remove('copied'), 1200);
        });
    });

    function showCopyNotification() {
        let notif = document.createElement('div');
        notif.textContent = 'Текст скопирован';
        notif.style.position = 'fixed';
        notif.style.top = '30%';
        notif.style.left = '50%';
        notif.style.transform = 'translateX(-50%)';
        notif.style.background = '#0D1D41';
        notif.style.color = '#fff';
        notif.style.padding = '10px 24px';
        notif.style.borderRadius = '8px';
        notif.style.fontSize = '16px';
        notif.style.zIndex = '9999';
        notif.style.boxShadow = '0 2px 8px rgba(0,0,0,0.15)';
        notif.style.opacity = '0';
        notif.style.transition = 'opacity 0.2s';
        document.body.appendChild(notif);
        setTimeout(() => {
            notif.style.opacity = '1';
        }, 10);
        setTimeout(() => {
            notif.style.opacity = '0';
            setTimeout(() => document.body.removeChild(notif), 200);
        }, 1200);
    }
});