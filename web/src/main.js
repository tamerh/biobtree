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

import '@/highlight.pack.js';
import UseCases1 from './conf/UseCases1'
import UseCases3 from './conf/UseCases3'
import UseCases4 from './conf/UseCases4'
import UseCases from './conf/UseCases.js'


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
            usecases: null
        }
    },
    components: {
        App
    },
    template: '<App :xref_conf="this.xref_conf" :app_conf="this.app_conf" :app_model="this.model" :fetcher="this.fetcher" :usecases="this.usecases"/>',
    beforeMount() {
        var endpoint = "http://localhost:8888/ws/";

        this.fetcher = new Fetch(endpoint)
        var request = new XMLHttpRequest();
        request.open('GET', endpoint + 'meta/', false); // `false` makes the request synchronous
        //for all data
        // request.open('GET', 'https://www.ebi.ac.uk/~tgur/biobtree/meta.php', false); // `false` makes the request synchronous
        request.send(null);

        if (request.status === 200) {
            var meta = JSON.parse(request.responseText)
            this.xref_conf = meta.datasets
        }

        this.app_conf = AppConf;

        if (meta.appparams.builtinset) {
            if (meta.appparams.builtinset == "0") {
                this.usecases = UseCases;
            } else if (meta.appparams.builtinset == "1" || meta.appparams.builtinset == "2") {
                this.usecases = UseCases1;
            } else if (meta.appparams.builtinset == "3") {
                this.usecases = UseCases3;
            } else if (meta.appparams.builtinset == "4") {
                this.usecases = UseCases4;
            }
        }

        this.model = new Model(this.fetcher, this.xref_conf, this.app_conf);
    }
})