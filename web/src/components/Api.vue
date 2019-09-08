<template>
  <div class="modal" v-bind:class="{ 'is-active' : apiActive}">
    <div class="modal-background" @click="$emit('close-api')"></div>
    <div class="modal-card">
      <header class="modal-card-head is-radiusless" style="padding:10px">
        <p class="modal-card-title has-text-centered">API</p>
      </header>
      <section class="modal-card-body">
        <div class="content">
          <p>
            XrefMap has an API with following functions. Usage example with javascript can be found
            <a>here</a>.
          </p>
          <h4>Search</h4>
          <p>Search calls take list of comma separated identifiers as a parameter. Example,</p>
          <div class="box has-background-white-bis exBox">
            <p>www.ebi.ac.uk/~tgur/xrefmap/api.php?ids=P15498</p>
          </div>

          <h4>Pagination</h4>
          <p>Each identifier search can have maximum 200 cross reference results. If it has more results, they are retrieved via pagination call. Pagination index starts from 0. Total page count is required to be specified with pageTotal parameter. This value is calculated via main search call. Example,</p>
          <div class="box has-background-white-bis exBox">
            <p>www.ebi.ac.uk/~tgur/xrefmap/ws/api.php?ids=P15498&src=8&page=0&pageTotal=2</p>
          </div>
          <h4>Filtering</h4>
          <p>Results can be filtered based on datasets. Example,</p>
          <div class="box has-background-white-bis exBox">
            <p>www.ebi.ac.uk/~tgur/ws/api.php?ids=P15498&src=8&filters=20,77</p>
          </div>

          <p>For pagination and filtering calls, dataset identifiers needs to be specified. Dataset identifiers are accessed from main search response.</p>

          <p>
            <strong>Limitations:</strong> API only supports binary response format. JSON format might be added in future.
            HTTP GET supported only, HTTP POST will be added later versions.
          </p>
        </div>
      </section>
      <!-- 
    <footer class="modal-card-foot">
      <button class="button is-success">Save changes</button>
      <button class="button">Cancel</button>
      </footer>-->
    </div>
    <button class="modal-close is-large" aria-label="close" @click="$emit('close-api')"></button>
  </div>
</template>

<script>
export default {
  name: "About",
  props: {
    apiActive: {
      type: Boolean,
      required: true
    }
  }
};
</script>
 
<style scoped>
</style>
