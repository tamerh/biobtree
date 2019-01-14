// The Vue build version to load with the `import` command
// (runtime-only or standalone) has been set in webpack.base.conf with an alias.
import Vue from 'vue'
import App from './App'
import Notifications from 'vue-notification'
import VueScrollTo from 'vue-scrollto'
import Multiselect from 'vue-multiselect'

Vue.config.productionTip = false

import Fetch from './data/Fetch.js'
import Model from './data/Model.js'
import AppConf from './conf/AppConf.js'

Vue.use(Notifications)


Vue.use(VueScrollTo, {
    container: "body",
    duration: 500,
    easing: "ease",
    offset: -50,
    cancelable: true,
    onStart: false,
    onDone: false,
    onCancel: false,
    x: false,
    y: true
})

Vue.component('multiselect', Multiselect)


/* eslint-disable no-new */
new Vue({
    el: '#app',
    data() {
        return {
            fetcher: null,
            xref_conf: null,
            model: null,
            app_conf: null,
        }
    },
    components: {
        App
    },
    template: '<App :xref_conf="this.xref_conf" :app_conf="this.app_conf" :app_model="this.model"/>',
    beforeMount() {
        this.fetcher = new Fetch()
        //this.xref_conf = XrefConf;

        var request = new XMLHttpRequest();
        request.open('GET', 'http://localhost:8888/ws/meta', false); // `false` makes the request synchronous
        request.send(null);

        if (request.status === 200) {
            console.log(request.responseText);
            this.xref_conf = JSON.parse(request.responseText)
        }

        this.app_conf = AppConf;
        this.model = new Model(this.fetcher, this.xref_conf, this.app_conf);
    }
})