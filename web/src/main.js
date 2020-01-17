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

import UseCases1 from './conf/UseCases1.json'
import UseCases3 from './conf/UseCases3.json'
import UseCases4 from './conf/UseCases4.json'
import UseCases from './conf/UseCases.json'

// trim function
if (typeof String.prototype.trim != 'function') { // detect native implementation
    String.prototype.trim = function () {
        return this.replace(/^\s+/, '').replace(/\s+$/, '');
    };
}


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
            usecases: {}
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
                this.usecases = {
                    'mix': UseCases.mix,
                    'gene': UseCases.gene,
                    'protein': UseCases.protein,
                    'chembl': UseCases.chembl,
                    'taxonomy&ontologies': UseCases.taxonomy
                };
            } else if (meta.appparams.builtinset == "1" || meta.appparams.builtinset == "2") {
                this.usecases = {
                    'mix': UseCases1.mix,
                    'gene': UseCases1.gene,
                    'protein': UseCases1.protein,
                    'taxonomy&ontologies': UseCases1.taxonomy
                };
            } else if (meta.appparams.builtinset == "3") {
                this.usecases = {
                    'mix': UseCases3.mix,
                    'protein': UseCases3.protein,
                    'taxonomy&ontologies': UseCases3.taxonomy
                };
            } else if (meta.appparams.builtinset == "4") {
                this.usecases = {
                    'mix': UseCases4.mix,
                    'protein': UseCases4.protein,
                    'chembl': UseCases4.chembl,
                    'taxonomy&ontologies': UseCases4.taxonomy
                };
            }
        }

        this.model = new Model(this.fetcher, this.xref_conf, this.app_conf);
    }
});